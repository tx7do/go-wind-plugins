package nacos

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"

	wind "github.com/tx7do/go-wind"
)

// ---------------------------------------------------------------------------
// Mock INamingClient — records calls and returns canned responses.
// ---------------------------------------------------------------------------

type mockNamingClient struct {
	naming_client.INamingClient

	registerParams   []vo.RegisterInstanceParam
	registerErr      error
	registerResult   bool
	deregisterParams []vo.DeregisterInstanceParam
	deregisterErr    error
	deregisterResult bool

	selectParams    []vo.SelectInstancesParam
	selectInstances []model.Instance
	selectErr       error

	getServiceParam  *vo.GetServiceParam
	getServiceResult model.Service
	getServiceErr    error

	subscribeParams  []*vo.SubscribeParam
	subscribeErr     error
	unsubscribeCalls int
}

func (m *mockNamingClient) RegisterInstance(param vo.RegisterInstanceParam) (bool, error) {
	m.registerParams = append(m.registerParams, param)
	if m.registerErr != nil {
		return false, m.registerErr
	}
	return m.registerResult, nil
}

func (m *mockNamingClient) DeregisterInstance(param vo.DeregisterInstanceParam) (bool, error) {
	m.deregisterParams = append(m.deregisterParams, param)
	if m.deregisterErr != nil {
		return false, m.deregisterErr
	}
	return m.deregisterResult, nil
}

func (m *mockNamingClient) SelectInstances(param vo.SelectInstancesParam) ([]model.Instance, error) {
	m.selectParams = append(m.selectParams, param)
	if m.selectErr != nil {
		return nil, m.selectErr
	}
	return m.selectInstances, nil
}

func (m *mockNamingClient) GetService(param vo.GetServiceParam) (model.Service, error) {
	m.getServiceParam = &param
	if m.getServiceErr != nil {
		return model.Service{}, m.getServiceErr
	}
	return m.getServiceResult, nil
}

func (m *mockNamingClient) Subscribe(param *vo.SubscribeParam) error {
	m.subscribeParams = append(m.subscribeParams, param)
	return m.subscribeErr
}

func (m *mockNamingClient) Unsubscribe(param *vo.SubscribeParam) error {
	m.unsubscribeCalls++
	return nil
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

func TestNew_Defaults(t *testing.T) {
	r := New(nil)

	if r.opts.prefix != "/microservices" {
		t.Errorf("prefix = %q, want /microservices", r.opts.prefix)
	}
	if r.opts.cluster != "DEFAULT" {
		t.Errorf("cluster = %q, want DEFAULT", r.opts.cluster)
	}
	if r.opts.group != "DEFAULT_GROUP" {
		t.Errorf("group = %q, want DEFAULT_GROUP", r.opts.group)
	}
	if r.opts.weight != 100 {
		t.Errorf("weight = %v, want 100", r.opts.weight)
	}
	if r.opts.kind != "grpc" {
		t.Errorf("kind = %q, want grpc", r.opts.kind)
	}
}

func TestOptions(t *testing.T) {
	r := New(nil,
		WithPrefix("/custom"),
		WithWeight(55.5),
		WithCluster("c-1"),
		WithGroup("GROUP-1"),
		WithDefaultKind("http"),
	)

	if r.opts.prefix != "/custom" {
		t.Errorf("WithPrefix: got %q, want /custom", r.opts.prefix)
	}
	if r.opts.weight != 55.5 {
		t.Errorf("WithWeight: got %v, want 55.5", r.opts.weight)
	}
	if r.opts.cluster != "c-1" {
		t.Errorf("WithCluster: got %q, want c-1", r.opts.cluster)
	}
	if r.opts.group != "GROUP-1" {
		t.Errorf("WithGroup: got %q, want GROUP-1", r.opts.group)
	}
	if r.opts.kind != "http" {
		t.Errorf("WithDefaultKind: got %q, want http", r.opts.kind)
	}
}

// ---------------------------------------------------------------------------
// Register — request building against the mock client
// ---------------------------------------------------------------------------

func TestRegister_EmptyNameFails(t *testing.T) {
	r := New(&mockNamingClient{})
	err := r.Register(context.Background(), &wind.Instance{
		Endpoints: []string{"grpc://127.0.0.1:9000"},
	})
	if !errors.Is(err, ErrServiceInstanceNameEmpty) {
		t.Errorf("Register() with empty name error = %v, want ErrServiceInstanceNameEmpty", err)
	}
}

func TestRegister_BuildsInstanceParam(t *testing.T) {
	cli := &mockNamingClient{}
	r := New(cli, WithGroup("G1"), WithCluster("C1"), WithWeight(42))

	inst := &wind.Instance{
		Name:      "helloworld",
		Version:   "1.2.3",
		Endpoints: []string{"grpc://127.0.0.1:9000"},
		Metadata:  map[string]string{"env": "test"},
	}
	if err := r.Register(context.Background(), inst); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if len(cli.registerParams) != 1 {
		t.Fatalf("RegisterInstance called %d times, want 1", len(cli.registerParams))
	}
	p := cli.registerParams[0]
	if p.ServiceName != "helloworld.grpc" {
		t.Errorf("ServiceName = %q, want helloworld.grpc (name.scheme)", p.ServiceName)
	}
	if p.Ip != "127.0.0.1" || p.Port != 9000 {
		t.Errorf("Ip/Port = %q/%d, want 127.0.0.1/9000", p.Ip, p.Port)
	}
	if p.Weight != 42 {
		t.Errorf("Weight = %v, want 42", p.Weight)
	}
	if !p.Enable || !p.Healthy || !p.Ephemeral {
		t.Errorf("Enable/Healthy/Ephemeral = %v/%v/%v, want all true", p.Enable, p.Healthy, p.Ephemeral)
	}
	if p.GroupName != "G1" || p.ClusterName != "C1" {
		t.Errorf("GroupName/ClusterName = %q/%q, want G1/C1", p.GroupName, p.ClusterName)
	}
	if p.Metadata["env"] != "test" || p.Metadata["kind"] != "grpc" || p.Metadata["version"] != "1.2.3" {
		t.Errorf("Metadata = %v, want env/kind/version carried over", p.Metadata)
	}
}

// A "weight" entry in the instance metadata overrides the option weight.
func TestRegister_MetadataWeightOverrides(t *testing.T) {
	cli := &mockNamingClient{}
	r := New(cli, WithWeight(10))

	inst := &wind.Instance{
		Name:      "helloworld",
		Endpoints: []string{"grpc://127.0.0.1:9000"},
		Metadata:  map[string]string{"weight": "77"},
	}
	if err := r.Register(context.Background(), inst); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if got := cli.registerParams[0].Weight; got != 77 {
		t.Errorf("Weight = %v, want 77 (metadata override)", got)
	}
}

// An unparsable metadata weight falls back to the option weight.
func TestRegister_InvalidMetadataWeightFallsBack(t *testing.T) {
	cli := &mockNamingClient{}
	r := New(cli, WithWeight(10))

	inst := &wind.Instance{
		Name:      "helloworld",
		Endpoints: []string{"grpc://127.0.0.1:9000"},
		Metadata:  map[string]string{"weight": "not-a-number"},
	}
	if err := r.Register(context.Background(), inst); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if got := cli.registerParams[0].Weight; got != 10 {
		t.Errorf("Weight = %v, want 10 (option fallback)", got)
	}
}

func TestRegister_DefaultsMetadataWhenNil(t *testing.T) {
	cli := &mockNamingClient{}
	r := New(cli)

	inst := &wind.Instance{
		Name:      "helloworld",
		Version:   "2.0.0",
		Endpoints: []string{"http://10.0.0.5:8080"},
	}
	if err := r.Register(context.Background(), inst); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	md := cli.registerParams[0].Metadata
	if md["kind"] != "http" || md["version"] != "2.0.0" {
		t.Errorf("Metadata = %v, want kind=http version=2.0.0", md)
	}
}

func TestRegister_BadEndpointFails(t *testing.T) {
	r := New(&mockNamingClient{})

	tests := []struct {
		name     string
		endpoint string
	}{
		{"unparsable", "://bad url"},
		{"no port", "grpc://127.0.0.1"},
		{"non-numeric port", "grpc://127.0.0.1:abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := r.Register(context.Background(), &wind.Instance{
				Name:      "helloworld",
				Endpoints: []string{tt.endpoint},
			})
			if err == nil {
				t.Errorf("Register() with %s endpoint should fail", tt.name)
			}
		})
	}
}

func TestRegister_ClientErrorWrapped(t *testing.T) {
	boom := errors.New("register refused")
	cli := &mockNamingClient{registerErr: boom}
	r := New(cli)

	err := r.Register(context.Background(), &wind.Instance{
		Name:      "helloworld",
		Endpoints: []string{"grpc://127.0.0.1:9000"},
	})
	// Register wraps the client error with %w so errors.Is matches, and the
	// message still carries the cause and the offending endpoint.
	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("Register() error = %v, want it to wrap the client error", err)
	}
	if !strings.Contains(err.Error(), "register refused") || !strings.Contains(err.Error(), "grpc://127.0.0.1:9000") {
		t.Errorf("Register() error = %v, want it to mention the cause and the endpoint", err)
	}
}

// ---------------------------------------------------------------------------
// Deregister — request building against the mock client
// ---------------------------------------------------------------------------

func TestDeregister_BuildsParam(t *testing.T) {
	cli := &mockNamingClient{}
	r := New(cli, WithGroup("G1"), WithCluster("C1"))

	inst := &wind.Instance{
		Name:      "helloworld",
		Endpoints: []string{"grpc://127.0.0.1:9000", "http://127.0.0.1:8080"},
	}
	if err := r.Deregister(context.Background(), inst); err != nil {
		t.Fatalf("Deregister() error = %v", err)
	}

	if len(cli.deregisterParams) != 2 {
		t.Fatalf("DeregisterInstance called %d times, want 2 (one per endpoint)", len(cli.deregisterParams))
	}
	p := cli.deregisterParams[0]
	if p.ServiceName != "helloworld.grpc" {
		t.Errorf("ServiceName = %q, want helloworld.grpc", p.ServiceName)
	}
	if p.Ip != "127.0.0.1" || p.Port != 9000 {
		t.Errorf("Ip/Port = %q/%d, want 127.0.0.1/9000", p.Ip, p.Port)
	}
	if p.GroupName != "G1" || p.Cluster != "C1" {
		t.Errorf("GroupName/Cluster = %q/%q, want G1/C1", p.GroupName, p.Cluster)
	}
	if !p.Ephemeral {
		t.Error("Ephemeral = false, want true")
	}
	if cli.deregisterParams[1].ServiceName != "helloworld.http" {
		t.Errorf("second ServiceName = %q, want helloworld.http", cli.deregisterParams[1].ServiceName)
	}
}

func TestDeregister_BadEndpointFails(t *testing.T) {
	r := New(&mockNamingClient{})

	if err := r.Deregister(context.Background(), &wind.Instance{
		Name:      "helloworld",
		Endpoints: []string{"127.0.0.1:8080"},
	}); err == nil {
		t.Error("Deregister() with an endpoint lacking a scheme should fail")
	}
}

func TestDeregister_ClientErrorPropagates(t *testing.T) {
	boom := errors.New("deregister refused")
	cli := &mockNamingClient{deregisterErr: boom}
	r := New(cli)

	err := r.Deregister(context.Background(), &wind.Instance{
		Name:      "helloworld",
		Endpoints: []string{"grpc://127.0.0.1:9000"},
	})
	if !errors.Is(err, boom) {
		t.Errorf("Deregister() error = %v, want %v", err, boom)
	}
}

// ---------------------------------------------------------------------------
// GetService — conversion from nacos instances
// ---------------------------------------------------------------------------

func TestGetService_ConvertsInstances(t *testing.T) {
	cli := &mockNamingClient{
		selectInstances: []model.Instance{
			{
				InstanceId:  "i1",
				ServiceName: "helloworld.grpc",
				Ip:          "10.0.0.1",
				Port:        9000,
				Weight:      33,
				Metadata:    map[string]string{"kind": "grpc", "version": "1.0.0"},
			},
			{
				InstanceId:  "i2",
				ServiceName: "helloworld.grpc",
				Ip:          "10.0.0.2",
				Port:        9001,
				Weight:      0, // falls back to the option weight
				Metadata:    map[string]string{"kind": "http", "version": "1.1.0"},
			},
		},
	}
	r := New(cli, WithGroup("G1"), WithDefaultKind("grpc"))

	got, err := r.GetService(context.Background(), "helloworld.grpc")
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetService() returned %d instances, want 2", len(got))
	}

	if got[0].ID != "i1" || got[0].Name != "helloworld.grpc" || got[0].Version != "1.0.0" {
		t.Errorf("instance[0] = %+v, want i1/helloworld.grpc/1.0.0", got[0])
	}
	if len(got[0].Endpoints) != 1 || got[0].Endpoints[0] != "grpc://10.0.0.1:9000" {
		t.Errorf("instance[0].Endpoints = %v, want [grpc://10.0.0.1:9000]", got[0].Endpoints)
	}
	// Weight is reported back into metadata as a rounded-up integer.
	if got[0].Metadata["weight"] != "33" {
		t.Errorf("instance[0].Metadata[weight] = %q, want 33", got[0].Metadata["weight"])
	}
	if got[1].Metadata["weight"] != "100" {
		t.Errorf("instance[1].Metadata[weight] = %q, want 100 (option default)", got[1].Metadata["weight"])
	}
	if got[1].Endpoints[0] != "http://10.0.0.2:9001" {
		t.Errorf("instance[1].Endpoints = %v, want [http://10.0.0.2:9001]", got[1].Endpoints)
	}

	if len(cli.selectParams) != 1 {
		t.Fatalf("SelectInstances called %d times, want 1", len(cli.selectParams))
	}
	sp := cli.selectParams[0]
	if sp.ServiceName != "helloworld.grpc" || sp.GroupName != "G1" || !sp.HealthyOnly {
		t.Errorf("SelectInstancesParam = %+v, want helloworld.grpc/G1/HealthyOnly", sp)
	}
}

func TestGetService_ErrorPropagates(t *testing.T) {
	boom := errors.New("select failed")
	cli := &mockNamingClient{selectErr: boom}
	r := New(cli)

	if _, err := r.GetService(context.Background(), "helloworld"); !errors.Is(err, boom) {
		t.Errorf("GetService() error = %v, want %v", err, boom)
	}
}

// ---------------------------------------------------------------------------
// Watcher — subscription params and event-driven Next
// ---------------------------------------------------------------------------

func TestWatch_SubscribesWithParams(t *testing.T) {
	cli := &mockNamingClient{}
	r := New(cli, WithGroup("G1"), WithCluster("C1"), WithDefaultKind("grpc"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w, err := r.Watch(ctx, "helloworld.grpc")
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	if len(cli.subscribeParams) != 1 {
		t.Fatalf("Subscribe called %d times, want 1", len(cli.subscribeParams))
	}
	p := cli.subscribeParams[0]
	if p.ServiceName != "helloworld.grpc" || p.GroupName != "G1" {
		t.Errorf("SubscribeParam = %+v, want helloworld.grpc/G1", p)
	}
	if len(p.Clusters) != 1 || p.Clusters[0] != "C1" {
		t.Errorf("Clusters = %v, want [C1]", p.Clusters)
	}
	if p.SubscribeCallback == nil {
		t.Error("SubscribeCallback = nil, want a callback")
	}
	_ = w
}

func TestWatch_Next_ConvertsHosts(t *testing.T) {
	cli := &mockNamingClient{
		getServiceResult: model.Service{
			Name: "helloworld.grpc",
			Hosts: []model.Instance{
				{InstanceId: "i1", Ip: "10.0.0.1", Port: 9000, Metadata: map[string]string{"kind": "grpc", "version": "1.0.0"}},
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w, err := newWatcher(ctx, cli, "helloworld.grpc", "G1", "grpc", []string{"C1"})
	if err != nil {
		t.Fatalf("newWatcher() error = %v", err)
	}

	// Fire the subscription callback; the watcher must wake up and fetch.
	w.subscribeParam.SubscribeCallback(nil, nil)

	got, err := w.Next(context.Background())
	if err != nil {
		t.Fatalf("watcher.Next() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "i1" || got[0].Version != "1.0.0" {
		t.Fatalf("watcher.Next() = %+v, want instance i1 at 1.0.0", got)
	}
	if got[0].Endpoints[0] != "grpc://10.0.0.1:9000" {
		t.Errorf("Endpoints = %v, want [grpc://10.0.0.1:9000]", got[0].Endpoints)
	}
	if cli.getServiceParam == nil || cli.getServiceParam.ServiceName != "helloworld.grpc" {
		t.Errorf("GetServiceParam = %+v, want helloworld.grpc", cli.getServiceParam)
	}
}

func TestWatch_Next_ErrorPropagates(t *testing.T) {
	boom := errors.New("get service failed")
	cli := &mockNamingClient{getServiceErr: boom}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w, err := newWatcher(ctx, cli, "svc", "DEFAULT_GROUP", "grpc", []string{"DEFAULT"})
	if err != nil {
		t.Fatalf("newWatcher() error = %v", err)
	}

	w.subscribeParam.SubscribeCallback(nil, nil)
	if _, err := w.Next(context.Background()); !errors.Is(err, boom) {
		t.Errorf("watcher.Next() error = %v, want %v", err, boom)
	}
}

// Next must report the watcher's own context cancellation (e.g. after Stop).
func TestWatch_Next_ContextCancelled(t *testing.T) {
	cli := &mockNamingClient{}

	ctx, cancel := context.WithCancel(context.Background())
	w, err := newWatcher(ctx, cli, "svc", "DEFAULT_GROUP", "grpc", []string{"DEFAULT"})
	if err != nil {
		t.Fatalf("newWatcher() error = %v", err)
	}

	// newWatcher seeds the channel with one initial signal; consume it so the
	// channel is empty before cancelling.
	if _, err := w.Next(context.Background()); err != nil {
		t.Fatalf("watcher.Next() before cancel error = %v", err)
	}

	cancel()
	if _, err := w.Next(context.Background()); !errors.Is(err, context.Canceled) {
		t.Errorf("watcher.Next() after cancel error = %v, want context.Canceled", err)
	}
}

func TestWatch_Stop_Unsubscribes(t *testing.T) {
	cli := &mockNamingClient{}
	r := New(cli)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w, err := r.Watch(ctx, "svc")
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	if err := w.Stop(); err != nil {
		t.Fatalf("watcher.Stop() error = %v", err)
	}
	if cli.unsubscribeCalls != 1 {
		t.Errorf("Unsubscribe called %d times, want 1", cli.unsubscribeCalls)
	}
}

func TestWatch_SubscribeErrorPropagates(t *testing.T) {
	boom := errors.New("subscribe refused")
	cli := &mockNamingClient{subscribeErr: boom}
	r := New(cli)

	if _, err := r.Watch(context.Background(), "svc"); !errors.Is(err, boom) {
		t.Errorf("Watch() error = %v, want %v", err, boom)
	}
}

// Compile-time guard: the mock must satisfy the naming client interface.
var _ naming_client.INamingClient = (*mockNamingClient)(nil)
