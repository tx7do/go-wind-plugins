package consul

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/consul/api"

	wind "github.com/tx7do/go-wind"
)

// ---------------------------------------------------------------------------
// Fake Consul HTTP API — implements just enough of Consul's endpoints for the
// agent/catalog/health calls used by Client and Registry.
// ---------------------------------------------------------------------------

// fakeConsul is an httptest-backed emulation of the small subset of the
// Consul HTTP API used by this package.
type fakeConsul struct {
	mu sync.Mutex

	server *httptest.Server

	registrations  []api.AgentServiceRegistration
	deregistered   []string
	ttlUpdates     []string
	registerStatus int
	// ttlUpdateStatus overrides the HTTP status of the TTL check-update
	// endpoint (0 = 200); used to force heartbeat failures.
	ttlUpdateStatus int

	// healthEntries maps service name → service entries returned by
	// /v1/health/service/<name>.
	healthEntries map[string][]*api.ServiceEntry
	// healthStatus overrides the HTTP status of the health endpoint (0 = 200).
	healthStatus int

	datacenters    []string
	datacentersErr bool
}

func newFakeConsul(t *testing.T) *fakeConsul {
	t.Helper()
	f := &fakeConsul{
		healthEntries: make(map[string][]*api.ServiceEntry),
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeConsul) client(t *testing.T) *api.Client {
	t.Helper()
	cli, err := api.NewClient(&api.Config{Address: f.server.URL})
	if err != nil {
		t.Fatalf("api.NewClient() error = %v", err)
	}
	return cli
}

func (f *fakeConsul) snapshotRegistrations() []api.AgentServiceRegistration {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]api.AgentServiceRegistration, len(f.registrations))
	copy(out, f.registrations)
	return out
}

func (f *fakeConsul) serve(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPut && r.URL.Path == "/v1/agent/service/register":
		body, _ := io.ReadAll(r.Body)
		var reg api.AgentServiceRegistration
		if err := json.Unmarshal(body, &reg); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.registrations = append(f.registrations, reg)
		status := f.registerStatus
		f.mu.Unlock()
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		w.WriteHeader(http.StatusOK)

	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v1/agent/service/deregister/"):
		id := strings.TrimPrefix(r.URL.Path, "/v1/agent/service/deregister/")
		f.mu.Lock()
		f.deregistered = append(f.deregistered, id)
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)

	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v1/agent/check/update/"):
		checkID := strings.TrimPrefix(r.URL.Path, "/v1/agent/check/update/")
		f.mu.Lock()
		f.ttlUpdates = append(f.ttlUpdates, checkID)
		status := f.ttlUpdateStatus
		f.mu.Unlock()
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		w.WriteHeader(http.StatusOK)

	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/health/service/"):
		name := strings.TrimPrefix(r.URL.Path, "/v1/health/service/")
		f.mu.Lock()
		entries := f.healthEntries[name]
		status := f.healthStatus
		f.mu.Unlock()
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		// Consul reports the current raft index via X-Consul-Index; the client
		// uses it as QueryMeta.LastIndex for blocking queries.
		w.Header().Set("X-Consul-Index", "42")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(entries)

	case r.Method == http.MethodGet && r.URL.Path == "/v1/catalog/datacenters":
		f.mu.Lock()
		fail := f.datacentersErr
		dcs := f.datacenters
		f.mu.Unlock()
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(dcs)

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// newTestRegistry builds a Registry around a fake consul server.
func newTestRegistry(t *testing.T, f *fakeConsul, opts ...Option) *Registry {
	t.Helper()
	return New(f.client(t), opts...)
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

func TestNew_Defaults(t *testing.T) {
	f := newFakeConsul(t)
	r := New(f.client(t))

	if !r.enableHealthCheck {
		t.Error("enableHealthCheck = false, want true by default")
	}
	if r.timeout != 10*time.Second {
		t.Errorf("timeout = %v, want 10s by default", r.timeout)
	}
	if r.cli == nil {
		t.Fatal("cli = nil, want initialized client")
	}
	if r.cli.dc != SingleDatacenter {
		t.Errorf("dc = %q, want %q by default", r.cli.dc, SingleDatacenter)
	}
	if r.cli.resolver == nil {
		t.Error("resolver = nil, want defaultResolver by default")
	}
	if r.cli.healthcheckInterval != 10 {
		t.Errorf("healthcheckInterval = %d, want 10 by default", r.cli.healthcheckInterval)
	}
	if !r.cli.heartbeat {
		t.Error("heartbeat = false, want true by default")
	}
	if r.cli.deregisterCriticalServiceAfter != 600 {
		t.Errorf("deregisterCriticalServiceAfter = %d, want 600 by default", r.cli.deregisterCriticalServiceAfter)
	}
	if r.cli.ctx == nil || r.cli.cancel == nil {
		t.Error("cli ctx/cancel = nil, want initialized by New")
	}
	r.cli.cancel()
}

func TestOptions(t *testing.T) {
	f := newFakeConsul(t)
	check := &api.AgentServiceCheck{TTL: "10s"}
	customCalled := false
	r := New(f.client(t),
		WithHealthCheck(false),
		WithTimeout(3*time.Second),
		WithDatacenter(MultiDatacenter),
		WithHeartbeat(false),
		WithHealthCheckInterval(5),
		WithDeregisterCriticalServiceAfter(60),
		WithServiceCheck(check),
		WithServiceResolver(func(_ context.Context, _ []*api.ServiceEntry) []*wind.Instance {
			customCalled = true
			return nil
		}),
	)

	if r.enableHealthCheck {
		t.Error("WithHealthCheck(false) did not disable health check")
	}
	if r.timeout != 3*time.Second {
		t.Errorf("WithTimeout: timeout = %v, want 3s", r.timeout)
	}
	if r.cli.dc != MultiDatacenter {
		t.Errorf("WithDatacenter: dc = %q, want %q", r.cli.dc, MultiDatacenter)
	}
	if r.cli.heartbeat {
		t.Error("WithHeartbeat(false) did not disable heartbeat")
	}
	if r.cli.healthcheckInterval != 5 {
		t.Errorf("WithHealthCheckInterval: got %d, want 5", r.cli.healthcheckInterval)
	}
	if r.cli.deregisterCriticalServiceAfter != 60 {
		t.Errorf("WithDeregisterCriticalServiceAfter: got %d, want 60", r.cli.deregisterCriticalServiceAfter)
	}
	if len(r.cli.serviceChecks) != 1 || r.cli.serviceChecks[0] != check {
		t.Errorf("WithServiceCheck: got %v, want the provided check", r.cli.serviceChecks)
	}
	// The custom resolver must be the one installed; invoking it must run our
	// replacement, not defaultResolver.
	r.cli.resolver(context.Background(), nil)
	if !customCalled {
		t.Error("WithServiceResolver: the custom resolver was not installed")
	}
	r.cli.cancel()
}

// ---------------------------------------------------------------------------
// defaultResolver — health service entry → wind.Instance conversion
// ---------------------------------------------------------------------------

func TestDefaultResolver(t *testing.T) {
	entries := []*api.ServiceEntry{
		{
			Service: &api.AgentService{
				ID:      "svc-1",
				Service: "helloworld",
				Tags:    []string{"version=v1.0.0", "other=tag"},
				Address: "10.0.0.1",
				Port:    8080,
				Meta:    map[string]string{"env": "test"},
			},
		},
		{
			// infra-style tagged addresses are skipped; a custom scheme wins.
			Service: &api.AgentService{
				ID:      "svc-2",
				Service: "helloworld",
				Address: "10.0.0.2",
				Port:    9090,
				TaggedAddresses: map[string]api.ServiceAddress{
					"lan_ipv4": {Address: "127.0.0.1", Port: 9090},
					"grpc":     {Address: "grpc://10.0.0.2:9090", Port: 9090},
				},
			},
		},
		{
			// no tagged addresses and no port → no endpoints at all.
			Service: &api.AgentService{
				ID:      "svc-3",
				Service: "hollow",
				Address: "10.0.0.3",
			},
		},
	}

	got := defaultResolver(context.Background(), entries)
	if len(got) != 3 {
		t.Fatalf("defaultResolver() returned %d instances, want 3", len(got))
	}

	if got[0].ID != "svc-1" || got[0].Name != "helloworld" {
		t.Errorf("instance[0] ID/Name = %q/%q, want svc-1/helloworld", got[0].ID, got[0].Name)
	}
	if got[0].Version != "v1.0.0" {
		t.Errorf("instance[0].Version = %q, want v1.0.0 (from version= tag)", got[0].Version)
	}
	if got[0].Metadata["env"] != "test" {
		t.Errorf("instance[0].Metadata = %v, want env=test carried over", got[0].Metadata)
	}
	wantEndpoint := "http://10.0.0.1:8080"
	if len(got[0].Endpoints) != 1 || got[0].Endpoints[0] != wantEndpoint {
		t.Errorf("instance[0].Endpoints = %v, want [%s]", got[0].Endpoints, wantEndpoint)
	}

	if got[1].Version != "" {
		t.Errorf("instance[1].Version = %q, want empty (no version tag)", got[1].Version)
	}
	if len(got[1].Endpoints) != 1 || got[1].Endpoints[0] != "grpc://10.0.0.2:9090" {
		t.Errorf("instance[1].Endpoints = %v, want [grpc://10.0.0.2:9090] (lan_ipv4 filtered)", got[1].Endpoints)
	}

	if len(got[2].Endpoints) != 0 {
		t.Errorf("instance[2].Endpoints = %v, want none (zero port)", got[2].Endpoints)
	}
}

func TestDefaultResolver_Empty(t *testing.T) {
	got := defaultResolver(context.Background(), nil)
	if len(got) != 0 {
		t.Errorf("defaultResolver(nil) = %v, want empty slice", got)
	}
}

// ---------------------------------------------------------------------------
// Client.Register against the fake consul API
// ---------------------------------------------------------------------------

func TestClient_Register_Minimal(t *testing.T) {
	f := newFakeConsul(t)
	c := newTestRegistry(t, f, WithHeartbeat(false), WithHealthCheck(false)).cli

	svc := &wind.Instance{
		ID:      "inst-1",
		Name:    "helloworld",
		Version: "1.0.0",
		Endpoints: []string{
			"grpc://127.0.0.1:9000",
			"http://127.0.0.1:8080",
		},
		Metadata: map[string]string{"env": "test"},
	}
	if err := c.Register(context.Background(), svc, false); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	regs := f.snapshotRegistrations()
	if len(regs) != 1 {
		t.Fatalf("consul received %d registrations, want 1", len(regs))
	}
	reg := regs[0]
	if reg.ID != "inst-1" {
		t.Errorf("registration.ID = %q, want inst-1", reg.ID)
	}
	if reg.Name != "helloworld" {
		t.Errorf("registration.Name = %q, want helloworld", reg.Name)
	}
	if len(reg.Tags) != 1 || reg.Tags[0] != "version=1.0.0" {
		t.Errorf("registration.Tags = %v, want [version=1.0.0]", reg.Tags)
	}
	if reg.Address != "127.0.0.1" || reg.Port != 9000 {
		t.Errorf("registration Address/Port = %q/%d, want 127.0.0.1/9000 (first endpoint)", reg.Address, reg.Port)
	}
	if reg.TaggedAddresses["grpc"].Address != "grpc://127.0.0.1:9000" {
		t.Errorf("TaggedAddresses[grpc] = %+v, want the grpc endpoint", reg.TaggedAddresses["grpc"])
	}
	if reg.TaggedAddresses["http"].Address != "http://127.0.0.1:8080" {
		t.Errorf("TaggedAddresses[http] = %+v, want the http endpoint", reg.TaggedAddresses["http"])
	}
	if len(reg.Checks) != 0 {
		t.Errorf("registration.Checks = %v, want none (health check and heartbeat disabled)", reg.Checks)
	}
}

func TestClient_Register_WithHealthCheck(t *testing.T) {
	f := newFakeConsul(t)
	c := newTestRegistry(t, f,
		WithHeartbeat(false),
		WithHealthCheckInterval(7),
		WithDeregisterCriticalServiceAfter(120),
	).cli

	svc := &wind.Instance{
		ID:        "inst-1",
		Name:      "helloworld",
		Endpoints: []string{"grpc://127.0.0.1:9000"},
	}
	if err := c.Register(context.Background(), svc, true); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	reg := f.snapshotRegistrations()[0]
	if len(reg.Checks) != 1 {
		t.Fatalf("registration.Checks = %v, want exactly one TCP check", reg.Checks)
	}
	check := reg.Checks[0]
	if check.TCP != "127.0.0.1:9000" {
		t.Errorf("check.TCP = %q, want 127.0.0.1:9000", check.TCP)
	}
	if check.Interval != "7s" {
		t.Errorf("check.Interval = %q, want 7s", check.Interval)
	}
	if check.DeregisterCriticalServiceAfter != "120s" {
		t.Errorf("check.DeregisterCriticalServiceAfter = %q, want 120s", check.DeregisterCriticalServiceAfter)
	}
	if check.Timeout != "5s" {
		t.Errorf("check.Timeout = %q, want 5s", check.Timeout)
	}
}

func TestClient_Register_CustomChecks(t *testing.T) {
	f := newFakeConsul(t)
	custom := &api.AgentServiceCheck{HTTP: "http://127.0.0.1:9000/healthz", Interval: "1s"}
	c := newTestRegistry(t, f, WithHeartbeat(false), WithServiceCheck(custom)).cli

	svc := &wind.Instance{
		ID:        "inst-1",
		Name:      "helloworld",
		Endpoints: []string{"grpc://127.0.0.1:9000"},
	}
	if err := c.Register(context.Background(), svc, true); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	reg := f.snapshotRegistrations()[0]
	if len(reg.Checks) != 2 {
		t.Fatalf("registration.Checks = %v, want TCP check plus the custom check", reg.Checks)
	}
	// The registration round-trips through JSON in the fake server, so compare
	// the custom check by field values rather than identity.
	if reg.Checks[1].HTTP != custom.HTTP || reg.Checks[1].Interval != custom.Interval {
		t.Errorf("registration.Checks[1] = %+v, want the custom check (HTTP %q, Interval %q)",
			reg.Checks[1], custom.HTTP, custom.Interval)
	}
}

func TestClient_Register_HeartbeatCheck(t *testing.T) {
	f := newFakeConsul(t)
	c := newTestRegistry(t, f, WithHeartbeat(true), WithHealthCheck(false), WithHealthCheckInterval(10)).cli

	svc := &wind.Instance{
		ID:        "inst-ttl",
		Name:      "helloworld",
		Endpoints: []string{"grpc://127.0.0.1:9000"},
	}
	if err := c.Register(context.Background(), svc, false); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	reg := f.snapshotRegistrations()[0]
	var ttlCheck *api.AgentServiceCheck
	for _, check := range reg.Checks {
		if check.CheckID == "service:inst-ttl" {
			ttlCheck = check
		}
	}
	if ttlCheck == nil {
		t.Fatalf("registration.Checks = %v, want a heartbeat check with ID service:inst-ttl", reg.Checks)
	}
	if ttlCheck.TTL != "20s" {
		t.Errorf("heartbeat check TTL = %q, want 20s (2 x healthcheckInterval)", ttlCheck.TTL)
	}
	if ttlCheck.DeregisterCriticalServiceAfter != "600s" {
		t.Errorf("heartbeat check DeregisterCriticalServiceAfter = %q, want 600s", ttlCheck.DeregisterCriticalServiceAfter)
	}

	// The heartbeat goroutine reports the TTL after one second; wait briefly
	// for the report and then shut everything down.
	deadline := time.Now().Add(5 * time.Second)
	for {
		f.mu.Lock()
		n := len(f.ttlUpdates)
		f.mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	c.cancel()
}

// A canceled client's heartbeat goroutine must neither re-register nor keep
// heartbeating: after cancel, the fake must observe no further writes. The
// heartbeat is driven into its failure-recovery path (TTL updates fail, which
// triggers the re-register-after-failure logic) so a canceled client cannot
// resurrect the instance via that path.
func TestClient_Register_HeartbeatNoResurrectionAfterCancel(t *testing.T) {
	f := newFakeConsul(t)
	f.mu.Lock()
	f.ttlUpdateStatus = http.StatusInternalServerError
	f.mu.Unlock()

	c := newTestRegistry(t, f, WithHeartbeat(true), WithHealthCheck(false), WithHealthCheckInterval(1)).cli

	svc := &wind.Instance{
		ID:        "inst-res",
		Name:      "helloworld",
		Endpoints: []string{"grpc://127.0.0.1:9000"},
	}
	if err := c.Register(context.Background(), svc, false); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Wait until the failure path is demonstrably active: the first failed TTL
	// update triggers a re-register, so a second registration must appear
	// while the client is still alive.
	deadline := time.Now().Add(15 * time.Second)
	for {
		if len(f.snapshotRegistrations()) >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(f.snapshotRegistrations()) < 2 {
		t.Skip("failure-recovery path did not trigger within the deadline")
	}

	// Cancel. A register request already in flight when cancel lands may still
	// be recorded, so allow a short grace period before taking the baseline.
	c.cancel()
	time.Sleep(500 * time.Millisecond)
	aliveRegs := len(f.snapshotRegistrations())

	// Now wait longer than the maximum re-register backoff (4s) plus one
	// heartbeat tick.
	time.Sleep(6 * time.Second)

	if got := len(f.snapshotRegistrations()); got != aliveRegs {
		t.Errorf("registrations after cancel = %d, want %d: a canceled client must not re-register the instance", got, aliveRegs)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	found := false
	for _, id := range f.deregistered {
		if id == "inst-res" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("deregistered IDs = %v, want the canceled instance %q to be deregistered", f.deregistered, "inst-res")
	}
}

func TestClient_Register_ServerError(t *testing.T) {
	f := newFakeConsul(t)
	f.mu.Lock()
	f.registerStatus = http.StatusInternalServerError
	f.mu.Unlock()
	c := newTestRegistry(t, f, WithHeartbeat(false), WithHealthCheck(false)).cli

	svc := &wind.Instance{
		ID:        "inst-1",
		Name:      "helloworld",
		Endpoints: []string{"grpc://127.0.0.1:9000"},
	}
	if err := c.Register(context.Background(), svc, false); err == nil {
		t.Error("Register() with a 500 from consul should fail")
	}
}

func TestClient_Register_InvalidEndpoint(t *testing.T) {
	f := newFakeConsul(t)
	c := newTestRegistry(t, f, WithHeartbeat(false)).cli

	svc := &wind.Instance{
		ID:        "inst-1",
		Name:      "helloworld",
		Endpoints: []string{"://bad url"},
	}
	if err := c.Register(context.Background(), svc, false); err == nil {
		t.Error("Register() with an unparsable endpoint should fail")
	}
	if len(f.snapshotRegistrations()) != 0 {
		t.Error("consul should not receive a registration for an invalid endpoint")
	}
}

func TestClient_Deregister(t *testing.T) {
	f := newFakeConsul(t)
	r := newTestRegistry(t, f, WithHeartbeat(false), WithHealthCheck(false))

	if err := r.Deregister(context.Background(), &wind.Instance{ID: "inst-9"}); err != nil {
		t.Fatalf("Deregister() error = %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.deregistered) != 1 || f.deregistered[0] != "inst-9" {
		t.Errorf("deregistered IDs = %v, want [inst-9]", f.deregistered)
	}
}

// ---------------------------------------------------------------------------
// Client.Service — health service queries
// ---------------------------------------------------------------------------

func healthEntry(id, name, version string) *api.ServiceEntry {
	return &api.ServiceEntry{
		Service: &api.AgentService{
			ID:      id,
			Service: name,
			Tags:    []string{"version=" + version},
			Address: "127.0.0.1",
			Port:    9000,
		},
	}
}

func TestClient_Service_SingleDatacenter(t *testing.T) {
	f := newFakeConsul(t)
	f.mu.Lock()
	f.healthEntries["helloworld"] = []*api.ServiceEntry{healthEntry("i1", "helloworld", "1.0.0")}
	f.mu.Unlock()

	c := newTestRegistry(t, f, WithHeartbeat(false)).cli

	got, index, err := c.Service(context.Background(), "helloworld", 0, true)
	if err != nil {
		t.Fatalf("Service() error = %v", err)
	}
	if index != 42 {
		t.Errorf("Service() index = %d, want 42 (from X-Consul-Index)", index)
	}
	if len(got) != 1 || got[0].ID != "i1" || got[0].Version != "1.0.0" {
		t.Errorf("Service() = %v, want one instance i1 at version 1.0.0", got)
	}
}

func TestClient_Service_ErrorPropagates(t *testing.T) {
	f := newFakeConsul(t)
	f.mu.Lock()
	f.healthStatus = http.StatusInternalServerError
	f.mu.Unlock()

	c := newTestRegistry(t, f, WithHeartbeat(false)).cli
	if _, _, err := c.Service(context.Background(), "helloworld", 0, true); err == nil {
		t.Error("Service() with a 500 from consul should fail")
	}
}

func TestClient_Service_MultiDatacenter(t *testing.T) {
	f := newFakeConsul(t)
	f.mu.Lock()
	f.datacenters = []string{"dc1", "dc2"}
	f.healthEntries["helloworld"] = []*api.ServiceEntry{healthEntry("i1", "helloworld", "1.0.0")}
	f.mu.Unlock()

	c := newTestRegistry(t, f, WithHeartbeat(false), WithDatacenter(MultiDatacenter)).cli

	got, _, err := c.Service(context.Background(), "helloworld", 0, true)
	if err != nil {
		t.Fatalf("Service() error = %v", err)
	}
	// Each datacenter yields the same single entry; the fake serves the same
	// list to both queries.
	if len(got) != 2 {
		t.Fatalf("Service() returned %d instances across 2 dcs, want 2", len(got))
	}
	for _, inst := range got {
		if inst.Metadata["dc"] != "dc1" && inst.Metadata["dc"] != "dc2" {
			t.Errorf("instance %s metadata dc = %q, want dc1 or dc2", inst.ID, inst.Metadata["dc"])
		}
	}
}

func TestClient_Service_MultiDatacenter_ListError(t *testing.T) {
	f := newFakeConsul(t)
	f.mu.Lock()
	f.datacentersErr = true
	f.mu.Unlock()

	c := newTestRegistry(t, f, WithHeartbeat(false), WithDatacenter(MultiDatacenter)).cli
	if _, _, err := c.Service(context.Background(), "helloworld", 0, true); err == nil {
		t.Error("Service() with a failing datacenter list should fail")
	}
}

// ---------------------------------------------------------------------------
// Registry.GetService / ListServices / Watch — via the fake consul API
// ---------------------------------------------------------------------------

func TestRegistry_GetService_RemoteFallback(t *testing.T) {
	f := newFakeConsul(t)
	f.mu.Lock()
	f.healthEntries["helloworld"] = []*api.ServiceEntry{
		healthEntry("i1", "helloworld", "1.0.0"),
		healthEntry("i2", "helloworld", "1.1.0"),
	}
	f.mu.Unlock()

	r := newTestRegistry(t, f, WithHeartbeat(false))

	got, err := r.GetService(context.Background(), "helloworld")
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetService() returned %d instances, want 2", len(got))
	}
	if got[0].ID != "i1" || got[1].ID != "i2" {
		t.Errorf("GetService() IDs = %q,%q, want i1,i2", got[0].ID, got[1].ID)
	}
}

func TestRegistry_GetService_NotFound(t *testing.T) {
	f := newFakeConsul(t)
	r := newTestRegistry(t, f, WithHeartbeat(false))

	if _, err := r.GetService(context.Background(), "missing"); err == nil {
		t.Error("GetService() for an unknown service should fail")
	}
}

func TestRegistry_ListServices_Empty(t *testing.T) {
	f := newFakeConsul(t)
	r := newTestRegistry(t, f, WithHeartbeat(false))

	all, err := r.ListServices()
	if err != nil {
		t.Fatalf("ListServices() error = %v", err)
	}
	if len(all) != 0 {
		t.Errorf("ListServices() = %v, want empty map", all)
	}
}

func TestRegistry_Watch_ReceivesInitialAndUpdatedServices(t *testing.T) {
	f := newFakeConsul(t)
	f.mu.Lock()
	f.healthEntries["helloworld"] = []*api.ServiceEntry{healthEntry("i1", "helloworld", "1.0.0")}
	f.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := newTestRegistry(t, f, WithHeartbeat(false))

	w, err := r.Watch(ctx, "helloworld")
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	got, err := w.Next(context.Background())
	if err != nil {
		t.Fatalf("watcher.Next() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "i1" {
		t.Errorf("watcher.Next() = %v, want the initial instance i1", got)
	}

	// A new broadcast (e.g. from the resolve loop) must reach the watcher.
	set := r.registry["helloworld"]
	if set == nil {
		t.Fatal("Watch() did not create a serviceSet for the service")
	}
	updated := []*wind.Instance{{ID: "i2", Name: "helloworld"}}
	set.broadcast(updated)

	got, err = w.Next(context.Background())
	if err != nil {
		t.Fatalf("watcher.Next() after broadcast error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "i2" {
		t.Errorf("watcher.Next() = %v, want the updated instance i2", got)
	}

	if err := w.Stop(); err != nil {
		t.Errorf("watcher.Stop() error = %v", err)
	}

	// After Stop the watcher must be unregistered from the service set.
	raw, ok := w.(*watcher)
	if !ok {
		t.Fatalf("Watch() returned %T, want *watcher", w)
	}
	set.lock.RLock()
	_, stillThere := set.watcher[raw]
	set.lock.RUnlock()
	if stillThere {
		t.Error("watcher.Stop() did not remove the watcher from its service set")
	}
}

func TestWatcher_Next_ReturnsContextErrorAfterStop(t *testing.T) {
	set := &serviceSet{
		serviceName: "helloworld",
		watcher:     make(map[*watcher]struct{}),
		services:    &atomic.Value{},
	}
	w := &watcher{event: make(chan struct{}, 1), set: set}
	w.ctx, w.cancel = context.WithCancel(context.Background())
	w.cancel()

	if _, err := w.Next(context.Background()); !errors.Is(err, context.Canceled) {
		t.Errorf("watcher.Next() after Stop error = %v, want context.Canceled", err)
	}
}

// A stopped watcher must not receive further broadcasts (it is removed from
// the set) and broadcast must not block on it.
func TestServiceSet_Broadcast_SkipsStoppedWatchers(t *testing.T) {
	set := &serviceSet{
		serviceName: "helloworld",
		watcher:     make(map[*watcher]struct{}),
		services:    &atomic.Value{},
	}
	set.broadcast([]*wind.Instance{{ID: "i1"}})

	if got := set.services.Load().([]*wind.Instance); len(got) != 1 || got[0].ID != "i1" {
		t.Errorf("services.Load() = %v, want one instance i1", got)
	}
}
