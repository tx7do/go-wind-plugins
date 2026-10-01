package polaris

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	wind "github.com/tx7do/go-wind"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/model"
)

// ---------------------------------------------------------------------------
// Mock polaris Instance — only the accessors used by the conversion helpers
// are implemented; the embedded nil interface covers the rest of the very
// large model.Instance interface.
// ---------------------------------------------------------------------------

type mockInstance struct {
	model.Instance

	id       string
	service  string
	host     string
	port     uint32
	metadata map[string]string
	healthy  bool
}

func (m *mockInstance) GetId() string                  { return m.id }
func (m *mockInstance) GetService() string             { return m.service }
func (m *mockInstance) GetHost() string                { return m.host }
func (m *mockInstance) GetPort() uint32                { return m.port }
func (m *mockInstance) GetMetadata() map[string]string { return m.metadata }
func (m *mockInstance) IsHealthy() bool                { return m.healthy }

// ---------------------------------------------------------------------------
// Mock polaris APIs — used to test Register/Deregister/GetService without a
// Polaris server.
// ---------------------------------------------------------------------------

type mockProviderAPI struct {
	api.ProviderAPI

	registerReq   *api.InstanceRegisterRequest
	registerResp  *model.InstanceRegisterResponse
	registerErr   error
	deregisterReq *api.InstanceDeRegisterRequest
	deregisterErr error
}

func (m *mockProviderAPI) Register(req *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
	m.registerReq = req
	if m.registerErr != nil {
		return nil, m.registerErr
	}
	return m.registerResp, nil
}

func (m *mockProviderAPI) Deregister(req *api.InstanceDeRegisterRequest) error {
	m.deregisterReq = req
	return m.deregisterErr
}

type mockConsumerAPI struct {
	api.ConsumerAPI

	instancesResp *model.InstancesResponse
	instancesErr  error
}

func (m *mockConsumerAPI) GetAllInstances(_ *api.GetAllInstancesRequest) (*model.InstancesResponse, error) {
	if m.instancesErr != nil {
		return nil, m.instancesErr
	}
	return m.instancesResp, nil
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

func TestNew_Defaults(t *testing.T) {
	r := New(nil, nil)

	want := options{
		Namespace: "default",
		Healthy:   true,
		Heartbeat: true,
	}
	if r.opt != want {
		t.Errorf("New() defaults = %+v, want %+v", r.opt, want)
	}
}

func TestWithNamespace(t *testing.T) {
	r := New(nil, nil, WithNamespace("prod"))
	if r.opt.Namespace != "prod" {
		t.Errorf("Namespace = %q, want prod", r.opt.Namespace)
	}
}

func TestWithServiceToken(t *testing.T) {
	r := New(nil, nil, WithServiceToken("tok"))
	if r.opt.ServiceToken != "tok" {
		t.Errorf("ServiceToken = %q, want tok", r.opt.ServiceToken)
	}
}

func TestWithProtocol(t *testing.T) {
	r := New(nil, nil, WithProtocol("grpc"))
	if r.opt.Protocol == nil || *r.opt.Protocol != "grpc" {
		t.Errorf("Protocol = %v, want pointer to grpc", r.opt.Protocol)
	}
}

func TestWithWeight(t *testing.T) {
	r := New(nil, nil, WithWeight(10))
	if r.opt.Weight != 10 {
		t.Errorf("Weight = %d, want 10", r.opt.Weight)
	}
}

func TestWithPriority(t *testing.T) {
	r := New(nil, nil, WithPriority(5))
	if r.opt.Priority != 5 {
		t.Errorf("Priority = %d, want 5", r.opt.Priority)
	}
}

func TestWithHealthy(t *testing.T) {
	r := New(nil, nil, WithHealthy(false))
	if r.opt.Healthy != false {
		t.Errorf("Healthy = %v, want false", r.opt.Healthy)
	}
}

func TestWithIsolate(t *testing.T) {
	r := New(nil, nil, WithIsolate(true))
	if r.opt.Isolate != true {
		t.Errorf("Isolate = %v, want true", r.opt.Isolate)
	}
}

func TestWithTTL(t *testing.T) {
	r := New(nil, nil, WithTTL(30))
	if r.opt.TTL != 30 {
		t.Errorf("TTL = %d, want 30", r.opt.TTL)
	}
}

func TestWithTimeout(t *testing.T) {
	r := New(nil, nil, WithTimeout(1_000_000_000))
	if r.opt.Timeout != 1_000_000_000 {
		t.Errorf("Timeout = %v, want 1s", r.opt.Timeout)
	}
}

func TestWithRetryCount(t *testing.T) {
	r := New(nil, nil, WithRetryCount(3))
	if r.opt.RetryCount != 3 {
		t.Errorf("RetryCount = %d, want 3", r.opt.RetryCount)
	}
}

func TestWithHeartbeat(t *testing.T) {
	r := New(nil, nil, WithHeartbeat(false))
	if r.opt.Heartbeat != false {
		t.Errorf("Heartbeat = %v, want false", r.opt.Heartbeat)
	}
}

func TestOptions_AppliedInOrder(t *testing.T) {
	r := New(nil, nil, WithNamespace("a"), WithNamespace("b"))
	if r.opt.Namespace != "b" {
		t.Errorf("Namespace = %q, want b (last option wins)", r.opt.Namespace)
	}
}

// ---------------------------------------------------------------------------
// Instance conversion helpers
// ---------------------------------------------------------------------------

func TestInstancesToServiceInstances_FiltersUnhealthy(t *testing.T) {
	instances := []model.Instance{
		&mockInstance{id: "i1", service: "svc", host: "10.0.0.1", port: 8080, healthy: true,
			metadata: map[string]string{"kind": "grpc", "version": "1.0.0"}},
		&mockInstance{id: "i2", service: "svc", host: "10.0.0.2", port: 8080, healthy: false},
	}

	got := instancesToServiceInstances(instances)
	if len(got) != 1 {
		t.Fatalf("instancesToServiceInstances() returned %d instances, want 1 (unhealthy filtered)", len(got))
	}
	if got[0].ID != "i1" {
		t.Errorf("instance ID = %q, want i1", got[0].ID)
	}
}

func TestInstanceToServiceInstance(t *testing.T) {
	inst := &mockInstance{
		id:      "inst-1",
		service: "helloworld",
		host:    "192.168.1.2",
		port:    8443,
		metadata: map[string]string{
			"kind":    "grpc",
			"version": "2.1.0",
		},
	}

	got := instanceToServiceInstance(inst)

	if got.ID != "inst-1" {
		t.Errorf("ID = %q, want inst-1", got.ID)
	}
	if got.Name != "helloworld" {
		t.Errorf("Name = %q, want helloworld", got.Name)
	}
	if got.Version != "2.1.0" {
		t.Errorf("Version = %q, want 2.1.0", got.Version)
	}
	wantEndpoint := "grpc://192.168.1.2:8443"
	if len(got.Endpoints) != 1 || got.Endpoints[0] != wantEndpoint {
		t.Errorf("Endpoints = %v, want [%s]", got.Endpoints, wantEndpoint)
	}
	if got.Metadata["kind"] != "grpc" || got.Metadata["version"] != "2.1.0" {
		t.Errorf("Metadata = %v, want kind/version carried over", got.Metadata)
	}
}

func TestInstanceToServiceInstance_MissingKind(t *testing.T) {
	got := instanceToServiceInstance(&mockInstance{
		id:       "inst-2",
		service:  "svc",
		host:     "h",
		port:     1,
		metadata: map[string]string{},
	})

	if got.Endpoints[0] != "://h:1" {
		t.Errorf("Endpoints = %v, want [://h:1] when metadata has no kind", got.Endpoints)
	}
}

// ---------------------------------------------------------------------------
// Register / Deregister / GetService via mocked APIs
// ---------------------------------------------------------------------------

func testRegistry(provider api.ProviderAPI, consumer api.ConsumerAPI, opts ...Option) *Registry {
	return New(provider, consumer, opts...)
}

func TestRegister_SetsInstanceIDAndMetadata(t *testing.T) {
	provider := &mockProviderAPI{
		registerResp: &model.InstanceRegisterResponse{InstanceID: "regid1"},
	}
	r := testRegistry(provider, nil, WithNamespace("ns-1"), WithServiceToken("tk"), WithHeartbeat(false))

	inst := &wind.Instance{
		Name:      "svc",
		Version:   "1.2.3",
		Endpoints: []string{"grpc://127.0.0.1:9000"},
		Metadata:  map[string]string{"env": "test"},
	}

	if err := r.Register(context.Background(), inst); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if inst.ID != "regid1" {
		t.Errorf("instance.ID = %q, want regid1", inst.ID)
	}

	req := provider.registerReq
	if req == nil {
		t.Fatal("provider.Register was not called")
	}
	if req.Service != "svcgrpc" {
		t.Errorf("Service = %q, want svcgrpc (name + scheme)", req.Service)
	}
	if req.Namespace != "ns-1" {
		t.Errorf("Namespace = %q, want ns-1", req.Namespace)
	}
	if req.ServiceToken != "tk" {
		t.Errorf("ServiceToken = %q, want tk", req.ServiceToken)
	}
	if req.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want 127.0.0.1", req.Host)
	}
	if req.Port != 9000 {
		t.Errorf("Port = %d, want 9000", req.Port)
	}
	// Metadata must be the user metadata plus kind/version.
	if req.Metadata["env"] != "test" {
		t.Errorf("Metadata[env] = %q, want test", req.Metadata["env"])
	}
	if req.Metadata["kind"] != "grpc" {
		t.Errorf("Metadata[kind] = %q, want grpc", req.Metadata["kind"])
	}
	if req.Metadata["version"] != "1.2.3" {
		t.Errorf("Metadata[version] = %q, want 1.2.3", req.Metadata["version"])
	}
}

func TestRegister_MultipleEndpointsJoinsID(t *testing.T) {
	calls := 0
	p := &seqProvider{calls: &calls, ids: []string{"id-a", "id-b"}}

	r := testRegistry(p, nil, WithHeartbeat(false))
	inst := &wind.Instance{
		Name:      "svc",
		Endpoints: []string{"grpc://127.0.0.1:1", "grpc://127.0.0.2:2"},
	}

	if err := r.Register(context.Background(), inst); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if inst.ID != "id-a|id-b" {
		t.Errorf("instance.ID = %q, want id-a|id-b (joined with %q)", inst.ID, _instanceIDSeparator)
	}
	if calls != 2 {
		t.Errorf("provider.Register called %d times, want 2", calls)
	}
}

// seqProvider returns a distinct instance ID per Register call.
type seqProvider struct {
	api.ProviderAPI
	calls *int
	ids   []string
}

func (s *seqProvider) Register(_ *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
	n := *s.calls
	if n >= len(s.ids) {
		return nil, errors.New("too many registrations")
	}
	*s.calls = n + 1
	return &model.InstanceRegisterResponse{InstanceID: s.ids[n]}, nil
}

func TestRegister_MissingPortFails(t *testing.T) {
	r := testRegistry(&mockProviderAPI{}, nil)
	inst := &wind.Instance{
		Name:      "svc",
		Endpoints: []string{"grpc://127.0.0.1"}, // no port
	}
	if err := r.Register(context.Background(), inst); err == nil {
		t.Error("Register() with a portless endpoint should fail")
	}
}

func TestRegister_InvalidEndpointFails(t *testing.T) {
	r := testRegistry(&mockProviderAPI{}, nil)
	inst := &wind.Instance{
		Name:      "svc",
		Endpoints: []string{"://bad url"},
	}
	if err := r.Register(context.Background(), inst); err == nil {
		t.Error("Register() with an unparsable endpoint should fail")
	}
}

func TestDeregister_PassesInstanceID(t *testing.T) {
	provider := &mockProviderAPI{}
	r := testRegistry(provider, nil)

	inst := &wind.Instance{
		Name:      "svc",
		Endpoints: []string{"grpc://127.0.0.1:9000"},
		ID:        "regid1",
	}

	if err := r.Deregister(context.Background(), inst); err != nil {
		t.Fatalf("Deregister() error = %v", err)
	}

	req := provider.deregisterReq
	if req == nil {
		t.Fatal("provider.Deregister was not called")
	}
	if req.InstanceID != "regid1" {
		t.Errorf("InstanceID = %q, want regid1", req.InstanceID)
	}
	if req.Service != "svcgrpc" {
		t.Errorf("Service = %q, want svcgrpc", req.Service)
	}
	if req.Host != "127.0.0.1" || req.Port != 9000 {
		t.Errorf("Host/Port = %q/%d, want 127.0.0.1/9000", req.Host, req.Port)
	}
}

func TestDeregister_ErrorPropagates(t *testing.T) {
	boom := errors.New("deregister failed")
	provider := &mockProviderAPI{deregisterErr: boom}
	r := testRegistry(provider, nil)

	inst := &wind.Instance{
		Name:      "svc",
		Endpoints: []string{"grpc://127.0.0.1:9000"},
		ID:        "regid1",
	}
	if err := r.Deregister(context.Background(), inst); !errors.Is(err, boom) {
		t.Errorf("Deregister() error = %v, want %v", err, boom)
	}
}

// Polaris instance IDs are UUID-style and contain "-"; the joined ID must
// survive a register→deregister round-trip without being truncated.
func TestDeregister_UUIDStyleIDs(t *testing.T) {
	provider := &mockProviderAPI{}
	r := testRegistry(provider, nil)

	idA := "8f3e1a2b-9c4d-5e6f-a1b2-c3d4e5f6a7b8"
	idB := "0a1b2c3d-4e5f-6071-8293-a4b5c6d7e8f9"

	inst := &wind.Instance{
		Name:      "svc",
		Endpoints: []string{"grpc://127.0.0.1:9000", "grpc://127.0.0.2:9001"},
		ID:        idA + _instanceIDSeparator + idB,
	}

	if err := r.Deregister(context.Background(), inst); err != nil {
		t.Fatalf("Deregister() error = %v", err)
	}

	if provider.deregisterReq == nil || provider.deregisterReq.InstanceID != idB {
		t.Errorf("last Deregister InstanceID = %v, want %q", provider.deregisterReq, idB)
	}
}

// Deregister must reject an ID whose per-endpoint count does not match the
// endpoint list instead of panicking on an out-of-range index.
func TestDeregister_IDEndpointCountMismatch(t *testing.T) {
	r := testRegistry(&mockProviderAPI{}, nil)

	inst := &wind.Instance{
		Name:      "svc",
		Endpoints: []string{"grpc://127.0.0.1:9000", "grpc://127.0.0.2:9001"},
		ID:        "only-one-id",
	}
	if err := r.Deregister(context.Background(), inst); err == nil {
		t.Error("Deregister() with mismatched ID count should fail")
	}
}

// heartbeatProvider records Heartbeat calls.
type heartbeatProvider struct {
	api.ProviderAPI

	mu    sync.Mutex
	calls int
}

func (m *heartbeatProvider) Register(_ *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
	return &model.InstanceRegisterResponse{InstanceID: "hb-inst"}, nil
}

func (m *heartbeatProvider) Deregister(_ *api.InstanceDeRegisterRequest) error { return nil }

func (m *heartbeatProvider) Heartbeat(_ *api.InstanceHeartbeatRequest) error {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	return nil
}

func (m *heartbeatProvider) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// With the default options (Heartbeat on, TTL 0) Register must not panic in
// the heartbeat setup (time.NewTicker panics on a non-positive period) and
// must register a stop signal for the endpoint.
func TestRegister_HeartbeatDefaultTTLNoPanic(t *testing.T) {
	r := testRegistry(&mockProviderAPI{
		registerResp: &model.InstanceRegisterResponse{InstanceID: "hb-1"},
	}, nil) // Heartbeat defaults to true, TTL defaults to 0

	inst := &wind.Instance{
		Name:      "svc",
		Endpoints: []string{"grpc://127.0.0.1:9000"},
	}
	if err := r.Register(context.Background(), inst); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	r.mu.Lock()
	_, ok := r.stops["grpc://127.0.0.1:9000"]
	r.mu.Unlock()
	if !ok {
		t.Error("Register() with Heartbeat should register a heartbeat stop signal")
	}

	// Clean up the heartbeat goroutine.
	if err := r.Deregister(context.Background(), inst); err != nil {
		t.Fatalf("Deregister() error = %v", err)
	}
	r.mu.Lock()
	_, ok = r.stops["grpc://127.0.0.1:9000"]
	r.mu.Unlock()
	if ok {
		t.Error("Deregister() should remove the heartbeat stop signal")
	}
}

// The heartbeat loop must report at least once per TTL period and must stop
// reporting after Deregister.
func TestRegister_HeartbeatStopsOnDeregister(t *testing.T) {
	provider := &heartbeatProvider{}
	r := testRegistry(provider, nil, WithTTL(1))

	inst := &wind.Instance{
		Name:      "svc",
		Endpoints: []string{"grpc://127.0.0.1:9000"},
	}
	if err := r.Register(context.Background(), inst); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Wait for at least one heartbeat report (TTL 1s).
	deadline := time.Now().Add(4 * time.Second)
	for provider.callCount() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("heartbeat loop did not report within 4s of registration")
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := r.Deregister(context.Background(), inst); err != nil {
		t.Fatalf("Deregister() error = %v", err)
	}

	// Give a still-running loop time to (wrongly) report again.
	time.Sleep(1500 * time.Millisecond)
	if got := provider.callCount(); got > 1 {
		t.Errorf("heartbeat reported %d times after Deregister, want at most 1", got)
	}
}

func TestGetService_ConvertsInstances(t *testing.T) {
	consumer := &mockConsumerAPI{
		instancesResp: &model.InstancesResponse{
			Instances: []model.Instance{
				&mockInstance{id: "i1", service: "svc-a", host: "10.0.0.1", port: 8080, healthy: true,
					metadata: map[string]string{"kind": "grpc", "version": "1.0.0"}},
				&mockInstance{id: "i2", service: "svc-a", host: "10.0.0.2", port: 8080, healthy: false},
			},
		},
	}
	r := testRegistry(nil, consumer, WithNamespace("ns-x"))

	instances, err := r.GetService(context.Background(), "svc-a")
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("GetService() returned %d instances, want 1 (unhealthy filtered)", len(instances))
	}
	if instances[0].ID != "i1" || instances[0].Name != "svc-a" {
		t.Errorf("GetService()[0] = %+v, want i1/svc-a", instances[0])
	}
}

func TestGetService_ErrorPropagates(t *testing.T) {
	boom := errors.New("query failed")
	consumer := &mockConsumerAPI{instancesErr: boom}
	r := testRegistry(nil, consumer)

	if _, err := r.GetService(context.Background(), "svc-a"); !errors.Is(err, boom) {
		t.Errorf("GetService() error = %v, want %v", err, boom)
	}
}
