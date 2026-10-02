package kubernetes

import (
	"context"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ---------------------------------------------------------------------------
// options
// ---------------------------------------------------------------------------

func TestNew_Defaults(t *testing.T) {
	src := New()
	if src == nil {
		t.Fatal("New() returned nil")
	}
	if src.opts.Namespace != "" {
		t.Errorf("default Namespace = %q, want empty", src.opts.Namespace)
	}
	if src.opts.LabelSelector != "" || src.opts.FieldSelector != "" {
		t.Errorf("default selectors not empty: %+v", src.opts)
	}
	if src.opts.KubeConfig != "" || src.opts.Master != "" {
		t.Errorf("default KubeConfig/Master not empty: %+v", src.opts)
	}
	if src.client != nil {
		t.Error("New() must not create a client eagerly")
	}
}

func TestOptions_Setters(t *testing.T) {
	op := options{}
	WithNamespace("ns1")(&op)
	WithLabelSelector("app=test")(&op)
	WithFieldSelector("field=x")(&op)
	WithKubeConfig("/path/kubeconfig")(&op)
	WithMaster("https://master:6443")(&op)

	if op.Namespace != "ns1" {
		t.Errorf("Namespace = %q, want ns1", op.Namespace)
	}
	if op.LabelSelector != "app=test" {
		t.Errorf("LabelSelector = %q, want app=test", op.LabelSelector)
	}
	if op.FieldSelector != "field=x" {
		t.Errorf("FieldSelector = %q, want field=x", op.FieldSelector)
	}
	if op.KubeConfig != "/path/kubeconfig" {
		t.Errorf("KubeConfig = %q, want /path/kubeconfig", op.KubeConfig)
	}
	if op.Master != "https://master:6443" {
		t.Errorf("Master = %q, want https://master:6443", op.Master)
	}
}

// ---------------------------------------------------------------------------
// resolveKey
// ---------------------------------------------------------------------------

func TestResolveKey(t *testing.T) {
	tests := []struct {
		name      string
		defaultNS string
		key       string
		wantNS    string
		wantName  string
		wantData  string
	}{
		{
			name:   "full key namespace/name/dataKey",
			key:    "ns1/app/config.json",
			wantNS: "ns1", wantName: "app", wantData: "config.json",
		},
		{
			name:   "two parts namespace/name",
			key:    "ns1/app",
			wantNS: "ns1", wantName: "app", wantData: "",
		},
		{
			name:      "bare name uses configured namespace",
			defaultNS: "cfg-ns",
			key:       "app",
			wantNS:    "cfg-ns", wantName: "app", wantData: "",
		},
		{
			name:   "bare name with empty namespace",
			key:    "app",
			wantNS: "", wantName: "app", wantData: "",
		},
		{
			name:   "dataKey containing slashes survives",
			key:    "ns1/app/dir/file.json",
			wantNS: "ns1", wantName: "app", wantData: "dir/file.json",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := &kube{opts: options{Namespace: tt.defaultNS}}
			ns, name, dataKey := k.resolveKey(tt.key)
			if ns != tt.wantNS || name != tt.wantName || dataKey != tt.wantData {
				t.Errorf("resolveKey(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tt.key, ns, name, dataKey, tt.wantNS, tt.wantName, tt.wantData)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// configMapData
// ---------------------------------------------------------------------------

func TestConfigMapData_SpecificKey(t *testing.T) {
	k := &kube{}
	cm := v1.ConfigMap{
		Data: map[string]string{"a.yaml": "aaa", "b.yaml": "bbb"},
	}
	got, err := k.configMapData(cm, "b.yaml")
	if err != nil {
		t.Fatalf("configMapData() error = %v", err)
	}
	if string(got) != "bbb" {
		t.Errorf("configMapData() = %q, want %q", string(got), "bbb")
	}
}

func TestConfigMapData_MissingKey(t *testing.T) {
	k := &kube{}
	cm := v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "cm1"},
		Data:       map[string]string{"a.yaml": "aaa"},
	}
	_, err := k.configMapData(cm, "nope.yaml")
	if err == nil {
		t.Fatal("configMapData() expected error for missing key")
	}
	for _, want := range []string{"nope.yaml", "ns1", "cm1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}

func TestConfigMapData_MergeAll(t *testing.T) {
	k := &kube{}
	cm := v1.ConfigMap{
		Data: map[string]string{"a": "one", "b": "two"},
	}
	got, err := k.configMapData(cm, "")
	if err != nil {
		t.Fatalf("configMapData() error = %v", err)
	}
	// Map iteration order is random; both entries must appear joined by newline.
	joined := string(got)
	if !(joined == "one\ntwo" || joined == "two\none") {
		t.Errorf("configMapData() = %q, want the two entries joined by newline", joined)
	}
}

func TestConfigMapData_MergeSingle(t *testing.T) {
	k := &kube{}
	cm := v1.ConfigMap{Data: map[string]string{"only": "value"}}
	got, err := k.configMapData(cm, "")
	if err != nil {
		t.Fatalf("configMapData() error = %v", err)
	}
	if string(got) != "value" {
		t.Errorf("configMapData() = %q, want %q", string(got), "value")
	}
}

// ---------------------------------------------------------------------------
// init / Load / WatchValue error paths (no cluster required)
// ---------------------------------------------------------------------------

func TestLoad_BadKubeConfigPath(t *testing.T) {
	src := New(WithKubeConfig("/nonexistent/kubeconfig-file"))
	_, err := src.Load(context.Background(), "ns/name")
	if err == nil {
		t.Fatal("Load() expected error for a bad kubeconfig path")
	}
}

func TestWatchValue_BadKubeConfigPath(t *testing.T) {
	src := New(WithKubeConfig("/nonexistent/kubeconfig-file"))
	_, err := src.WatchValue(context.Background(), "ns/name")
	if err == nil {
		t.Fatal("WatchValue() expected error for a bad kubeconfig path")
	}
}

func TestLoad_InClusterMissingNamespace(t *testing.T) {
	// Force the in-cluster path to fail deterministically by clearing the
	// service host/port environment variables.
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")

	src := New()
	_, err := src.Load(context.Background(), "name-only")
	if err == nil {
		t.Fatal("Load() expected in-cluster init error outside a cluster")
	}
	if src.client != nil {
		t.Error("client should remain nil after a failed init")
	}
}

func TestInit_BadMasterURL(t *testing.T) {
	src := New(WithKubeConfig("/nonexistent/kubeconfig"), WithMaster("://bad url"))
	if err := src.init(); err == nil {
		t.Fatal("init() expected error for a bad kubeconfig/master combination")
	}
}
