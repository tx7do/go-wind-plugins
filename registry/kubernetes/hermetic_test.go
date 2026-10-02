package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	wind "github.com/tx7do/go-wind"
)

// ---------------------------------------------------------------------------
// Small pure helpers
// ---------------------------------------------------------------------------

// New cannot return an error (its signature returns only *Registry), so a nil
// clientSet must fail fast with a clear panic instead of a cryptic nil
// pointer dereference later inside the informer goroutines or in Register.
func TestNew_NilClientSetPanicsEarly(t *testing.T) {
	defer func() {
		rec := recover()
		if rec == nil {
			t.Error("New(nil, ...) must panic immediately instead of deferring the failure to Start/Register")
			return
		}
		msg := fmt.Sprint(rec)
		if !strings.Contains(msg, "clientSet") {
			t.Errorf("panic message = %q, want it to mention clientSet", msg)
		}
	}()
	New(nil, metav1.NamespaceDefault)
}

func TestIsEmptyObjectString(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", true},
		{"{}", true},
		{"null", true},
		{"nil", true},
		{"[]", true},
		{`{"a":"b"}`, false},
		{"x", false},
	}
	for _, tt := range tests {
		if got := isEmptyObjectString(tt.in); got != tt.want {
			t.Errorf("isEmptyObjectString(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestMarshalUnmarshal_RoundTrip(t *testing.T) {
	in := map[string]string{"region": "sh", "zone": "sh001"}

	s, err := marshal(in)
	if err != nil {
		t.Fatalf("marshal() error = %v", err)
	}
	if s == "" {
		t.Fatal("marshal() returned an empty string")
	}

	out := map[string]string{}
	if err := unmarshal(s, &out); err != nil {
		t.Fatalf("unmarshal() error = %v", err)
	}
	if out["region"] != "sh" || out["zone"] != "sh001" {
		t.Errorf("unmarshal() = %v, want the original map", out)
	}

	if err := unmarshal("not json", &out); err == nil {
		t.Error("unmarshal() with invalid JSON should fail")
	}
}

func TestGetProtocolMapByEndpoints(t *testing.T) {
	m, err := getProtocolMapByEndpoints([]string{
		"grpc://10.0.0.1:9000",
		"http://10.0.0.1:80",
	})
	if err != nil {
		t.Fatalf("getProtocolMapByEndpoints() error = %v", err)
	}
	if m.GetProtocol(9000) != "grpc" {
		t.Errorf("GetProtocol(9000) = %q, want grpc", m.GetProtocol(9000))
	}
	if m.GetProtocol(80) != "http" {
		t.Errorf("GetProtocol(80) = %q, want http", m.GetProtocol(80))
	}
	if m.GetProtocol(1234) != "" {
		t.Errorf("GetProtocol(1234) = %q, want empty for unknown port", m.GetProtocol(1234))
	}
}

func TestGetProtocolMapByEndpoints_InvalidEndpoint(t *testing.T) {
	if _, err := getProtocolMapByEndpoints([]string{"://bad url"}); err == nil {
		t.Error("getProtocolMapByEndpoints() with an unparsable endpoint should fail")
	}
}

// ---------------------------------------------------------------------------
// Pod annotation decoding
// ---------------------------------------------------------------------------

func testPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-1",
			Name:      "pod-1",
			Labels: map[string]string{
				LabelsKeyServiceID:      "id-1",
				LabelsKeyServiceName:    "helloworld",
				LabelsKeyServiceVersion: "v1.2.3",
			},
			Annotations: map[string]string{
				AnnotationsKeyMetadata:    `{"region":"sh","zone":"sh001"}`,
				AnnotationsKeyProtocolMap: `{"8080":"grpc","80":"http"}`,
			},
		},
	}
}

func TestGetMetadataFromPod(t *testing.T) {
	md, err := getMetadataFromPod(testPod())
	if err != nil {
		t.Fatalf("getMetadataFromPod() error = %v", err)
	}
	if md["region"] != "sh" || md["zone"] != "sh001" {
		t.Errorf("metadata = %v, want region/zone decoded", md)
	}

	// Missing or empty annotation yields an empty map, not an error.
	pod := testPod()
	pod.Annotations = nil
	md, err = getMetadataFromPod(pod)
	if err != nil {
		t.Fatalf("getMetadataFromPod() without annotation error = %v", err)
	}
	if len(md) != 0 {
		t.Errorf("metadata = %v, want empty", md)
	}
}

func TestGetMetadataFromPod_InvalidJSON(t *testing.T) {
	pod := testPod()
	pod.Annotations = map[string]string{AnnotationsKeyMetadata: "not json"}

	_, err := getMetadataFromPod(pod)
	if err == nil {
		t.Fatal("getMetadataFromPod() with invalid JSON should fail")
	}
	var resErr *ErrorHandleResource
	if !errors.As(err, &resErr) {
		t.Fatalf("error = %T, want *ErrorHandleResource", err)
	}
	if resErr.Namespace != "ns-1" || resErr.Name != "pod-1" {
		t.Errorf("ErrorHandleResource = %+v, want namespace ns-1 and name pod-1", resErr)
	}
}

func TestGetProtocolMapFromPod(t *testing.T) {
	m, err := getProtocolMapFromPod(testPod())
	if err != nil {
		t.Fatalf("getProtocolMapFromPod() error = %v", err)
	}
	if m.GetProtocol(8080) != "grpc" || m.GetProtocol(80) != "http" {
		t.Errorf("protocolMap = %v, want 8080=grpc and 80=http", m)
	}

	pod := testPod()
	pod.Annotations = nil
	m, err = getProtocolMapFromPod(pod)
	if err != nil {
		t.Fatalf("getProtocolMapFromPod() without annotation error = %v", err)
	}
	if len(m) != 0 {
		t.Errorf("protocolMap = %v, want empty", m)
	}
}

func TestGetProtocolMapFromPod_InvalidJSON(t *testing.T) {
	pod := testPod()
	pod.Annotations = map[string]string{AnnotationsKeyProtocolMap: "{"}

	_, err := getProtocolMapFromPod(pod)
	if err == nil {
		t.Fatal("getProtocolMapFromPod() with invalid JSON should fail")
	}
	var resErr *ErrorHandleResource
	if !errors.As(err, &resErr) {
		t.Fatalf("error = %T, want *ErrorHandleResource", err)
	}
}

// ---------------------------------------------------------------------------
// Pod → wind.Instance conversion
// ---------------------------------------------------------------------------

func TestGetServiceInstanceFromPod(t *testing.T) {
	pod := testPod()
	pod.Status.PodIP = "10.1.2.3"
	pod.Spec.Containers = []corev1.Container{
		{
			Name: "app",
			Ports: []corev1.ContainerPort{
				{Name: "http", ContainerPort: 80},
				{ContainerPort: 9000}, // unnamed port, no protocol map entry
				{Name: "grpc-9001", ContainerPort: 9001, Protocol: corev1.ProtocolTCP},
			},
		},
	}

	got, err := getServiceInstanceFromPod(pod)
	if err != nil {
		t.Fatalf("getServiceInstanceFromPod() error = %v", err)
	}

	if got.ID != "id-1" || got.Name != "helloworld" || got.Version != "v1.2.3" {
		t.Errorf("identity = %q/%q/%q, want id-1/helloworld/v1.2.3", got.ID, got.Name, got.Version)
	}
	if got.Metadata["region"] != "sh" {
		t.Errorf("Metadata = %v, want region=sh decoded from annotations", got.Metadata)
	}

	// Port 80 has an explicit protocol map entry; port 9000 falls back to the
	// port name prefix; port 9001 has no name, so the IP protocol is used.
	want := []string{
		"http://10.1.2.3:80",
		"://10.1.2.3:9000",
		"grpc://10.1.2.3:9001",
	}
	if len(got.Endpoints) != len(want) {
		t.Fatalf("Endpoints = %v, want %v", got.Endpoints, want)
	}
	for i := range want {
		if got.Endpoints[i] != want[i] {
			t.Errorf("Endpoints[%d] = %q, want %q", i, got.Endpoints[i], want[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Iterator
// ---------------------------------------------------------------------------

func TestIterator_Next_ReceivesInstances(t *testing.T) {
	ch := make(chan []*wind.Instance, 1)
	stopCh := make(chan struct{})
	iter := NewIterator(ch, stopCh)

	want := []*wind.Instance{{ID: "i1", Name: "helloworld"}}
	ch <- want

	got, err := iter.Next(context.Background())
	if err != nil {
		t.Fatalf("Iterator.Next() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "i1" {
		t.Errorf("Iterator.Next() = %v, want instance i1", got)
	}
}

func TestIterator_Stop_ClosesAndIsIdempotent(t *testing.T) {
	ch := make(chan []*wind.Instance)
	stopCh := make(chan struct{})
	iter := NewIterator(ch, stopCh)

	if err := iter.Stop(); err != nil {
		t.Fatalf("Iterator.Stop() error = %v", err)
	}
	// A second Stop must not panic on the closed channel.
	if err := iter.Stop(); err != nil {
		t.Fatalf("second Iterator.Stop() error = %v", err)
	}

	if _, err := iter.Next(context.Background()); !errors.Is(err, ErrIteratorClosed) {
		t.Errorf("Iterator.Next() after Stop error = %v, want ErrIteratorClosed", err)
	}
}

// ---------------------------------------------------------------------------
// Runtime helpers
// ---------------------------------------------------------------------------

func TestGetPodName_UsesHostnameEnv(t *testing.T) {
	t.Setenv("HOSTNAME", "my-pod-abc")
	if got := GetPodName(); got != "my-pod-abc" {
		t.Errorf("GetPodName() = %q, want my-pod-abc", got)
	}
}

func TestGetNamespace_ReturnsLoadedNamespace(t *testing.T) {
	// LoadNamespace reads the in-cluster service account file, which does not
	// exist off-cluster, so the default must be empty.
	if LoadNamespace() != "" {
		t.Errorf("LoadNamespace() = %q, want empty outside a cluster", LoadNamespace())
	}

	old := currentNamespace
	currentNamespace = "custom-ns"
	defer func() { currentNamespace = old }()

	if got := GetNamespace(); got != "custom-ns" {
		t.Errorf("GetNamespace() = %q, want custom-ns", got)
	}
}

func TestErrorHandleResource_Message(t *testing.T) {
	err := &ErrorHandleResource{Namespace: "ns", Name: "pod", Reason: errors.New("boom")}
	want := "failed to handle resource(namespace=ns, name=pod): boom"
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

// ---------------------------------------------------------------------------
// GetService / Watch against a fake clientset-backed informer
// ---------------------------------------------------------------------------

// newFakeRegistry builds a Registry around a fake clientset. The concrete
// *kubernetes.Clientset parameter of New cannot accept a fake, so the
// registry is assembled from the same informer primitives New uses.
func newFakeRegistry(t *testing.T, pods ...runtime.Object) (*Registry, *fake.Clientset) {
	t.Helper()

	client := fake.NewSimpleClientset(pods...)
	factory := informers.NewSharedInformerFactoryWithOptions(client, 0, informers.WithNamespace(metav1.NamespaceAll))
	podInformer := factory.Core().V1().Pods().Informer()

	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })
	factory.Start(stopCh)

	// Bound the cache sync wait so a stalled informer cannot hang the test.
	synced := make(chan bool, 1)
	go func() { synced <- cache.WaitForCacheSync(stopCh, podInformer.HasSynced) }()
	select {
	case ok := <-synced:
		if !ok {
			t.Fatal("informer cache never synced")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the informer cache to sync")
	}

	return &Registry{
		clientSet:       nil,
		informerFactory: factory,
		podInformer:     podInformer,
		podLister:       factory.Core().V1().Pods().Lister(),
		stopCh:          make(chan struct{}),
	}, client
}

func runningPod(name, app string) *corev1.Pod {
	pod := testPod()
	pod.Namespace = "default"
	pod.Name = name
	pod.Labels = map[string]string{
		LabelsKeyServiceID:      name,
		LabelsKeyServiceName:    app,
		LabelsKeyServiceVersion: "v1.0.0",
	}
	pod.Status.Phase = corev1.PodRunning
	return pod
}

func TestRegistry_GetService_FiltersNonRunningPods(t *testing.T) {
	up := runningPod("pod-up", "helloworld")
	up.Status.PodIP = "10.0.0.1"

	pending := runningPod("pod-pending", "helloworld")
	pending.Status.Phase = corev1.PodPending

	r, _ := newFakeRegistry(t, up, pending)

	got, err := r.GetService(context.Background(), "helloworld")
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("GetService() returned %d instances, want 1 (non-running pods filtered)", len(got))
	}
	if got[0].ID != "pod-up" || got[0].Name != "helloworld" {
		t.Errorf("GetService()[0] = %+v, want pod-up/helloworld", got[0])
	}
}

func TestRegistry_GetService_UnknownService(t *testing.T) {
	r, _ := newFakeRegistry(t, runningPod("pod-up", "helloworld"))

	got, err := r.GetService(context.Background(), "missing")
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("GetService() = %v, want no instances for an unknown service", got)
	}
}

func TestRegistry_Watch_PushesInstancesOnPodCreate(t *testing.T) {
	r, client := newFakeRegistry(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w, err := r.Watch(ctx, "helloworld")
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	defer func() { _ = w.Stop() }()

	// Creating a matching pod must reach the watcher through the informer.
	pod := runningPod("pod-1", "helloworld")
	pod.Status.PodIP = "10.0.0.7"
	if _, err := client.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("fake pod create error = %v", err)
	}

	type nextResult struct {
		instances []*wind.Instance
		err       error
	}
	resCh := make(chan nextResult, 1)
	go func() {
		ins, err := w.Next(context.Background())
		resCh <- nextResult{ins, err}
	}()

	select {
	case res := <-resCh:
		if res.err != nil {
			t.Fatalf("watcher.Next() error = %v", res.err)
		}
		if len(res.instances) != 1 || res.instances[0].ID != "pod-1" {
			t.Errorf("watcher.Next() = %v, want instance pod-1", res.instances)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the watcher to observe the pod")
	}

	// Close must stop the registry-wide callbacks without blocking.
	r.Close()
}

func TestRegistry_Close_TwiceDoesNotPanic(t *testing.T) {
	r, _ := newFakeRegistry(t)
	r.Close()
	r.Close()
}

// Guard against accidental changes to the label/annotation key contract.
func TestFieldKeyConstants(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"service id", LabelsKeyServiceID, "wind-service-id"},
		{"service app", LabelsKeyServiceName, "wind-service-app"},
		{"service version", LabelsKeyServiceVersion, "wind-service-version"},
		{"metadata", AnnotationsKeyMetadata, "wind-service-metadata"},
		{"protocol map", AnnotationsKeyProtocolMap, "wind-service-protocols"},
		{"namespace path", ServiceAccountNamespacePath, "/var/run/secrets/kubernetes.io/serviceaccount/namespace"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s key = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}
