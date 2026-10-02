package servicecomb

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/go-chassis/cari/discovery"
	"github.com/go-chassis/sc-client"

	wind "github.com/tx7do/go-wind"
)

// ---------------------------------------------------------------------------
// Recording mock client — configurable per test case.
// ---------------------------------------------------------------------------

type recordingClient struct {
	mu sync.Mutex

	findInstancesErr error
	findInstances    []*pb.MicroServiceInstance
	findConsumerID   string
	findServiceName  string

	registerServiceResult string
	registerServiceErr    error

	getServiceIDResult string
	getServiceIDErr    error
	getServiceIDApp    string
	getServiceIDName   string
	getServiceIDVer    string
	getServiceIDEnv    string

	registerInstanceResult string
	registerInstanceErr    error
	registeredInstance     *pb.MicroServiceInstance

	heartbeatErr     error
	heartbeatCalls   int
	unregisterResult bool
	unregisterErr    error
	unregisterSID    string
	unregisterIID    string

	watchServiceID string
	watchCallback  func(*sc.MicroServiceInstanceChangedEvent)
	watchErr       error
}

func (c *recordingClient) FindMicroServiceInstances(consumerID, _, microServiceName, _ string, _ ...sc.CallOption) ([]*pb.MicroServiceInstance, error) {
	c.mu.Lock()
	c.findConsumerID = consumerID
	c.findServiceName = microServiceName
	err := c.findInstancesErr
	instances := c.findInstances
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return instances, nil
}

func (c *recordingClient) RegisterService(ms *pb.MicroService) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.registerServiceResult, c.registerServiceErr
}

func (c *recordingClient) GetMicroServiceID(appID, microServiceName, version, env string, _ ...sc.CallOption) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.getServiceIDApp, c.getServiceIDName, c.getServiceIDVer, c.getServiceIDEnv = appID, microServiceName, version, env
	return c.getServiceIDResult, c.getServiceIDErr
}

func (c *recordingClient) RegisterMicroServiceInstance(inst *pb.MicroServiceInstance) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.registeredInstance = inst
	return c.registerInstanceResult, c.registerInstanceErr
}

func (c *recordingClient) Heartbeat(_, _ string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.heartbeatCalls++
	return true, c.heartbeatErr
}

func (c *recordingClient) UnregisterMicroServiceInstance(sid, iid string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.unregisterSID, c.unregisterIID = sid, iid
	return c.unregisterResult, c.unregisterErr
}

func (c *recordingClient) WatchMicroService(sid string, cb func(*sc.MicroServiceInstanceChangedEvent)) error {
	c.mu.Lock()
	c.watchServiceID = sid
	c.watchCallback = cb
	err := c.watchErr
	c.mu.Unlock()
	return err
}

// registryExceptionFor builds a *sc.RegistryException whose Message is the
// JSON body the real service-center HTTP client produces.
func registryExceptionFor(code int32) *sc.RegistryException {
	return &sc.RegistryException{
		Title:   "registerMicroService failed",
		Message: `{"errorCode":"` + strconv.FormatInt(int64(code), 10) + `","errorMessage":"service error"}`,
	}
}

// ---------------------------------------------------------------------------
// GetService
// ---------------------------------------------------------------------------

func TestGetService_ConvertsInstanceFields(t *testing.T) {
	cli := &recordingClient{
		findInstances: []*pb.MicroServiceInstance{
			{
				InstanceId: "inst-1",
				ServiceId:  "svc-1",
				Properties: map[string]string{"env": "test"},
				Endpoints:  []string{"grpc://127.0.0.1:9000"},
			},
		},
	}
	r := New(cli)

	got, err := r.GetService(context.Background(), "helloworld")
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("GetService() returned %d instances, want 1", len(got))
	}
	inst := got[0]
	if inst.ID != "inst-1" {
		t.Errorf("ID = %q, want inst-1", inst.ID)
	}
	if inst.Name != "helloworld" {
		t.Errorf("Name = %q, want helloworld", inst.Name)
	}
	// The service-comb adapter reports ServiceId as the version.
	if inst.Version != "svc-1" {
		t.Errorf("Version = %q, want svc-1 (from ServiceId)", inst.Version)
	}
	if inst.Metadata["env"] != "test" {
		t.Errorf("Metadata = %v, want env=test carried over", inst.Metadata)
	}
	if len(inst.Endpoints) != 1 || inst.Endpoints[0] != "grpc://127.0.0.1:9000" {
		t.Errorf("Endpoints = %v, want [grpc://127.0.0.1:9000]", inst.Endpoints)
	}

	cli.mu.Lock()
	defer cli.mu.Unlock()
	if cli.findConsumerID != "" || cli.findServiceName != "helloworld" {
		t.Errorf("FindMicroServiceInstances(consumer=%q, name=%q), want empty consumer and helloworld", cli.findConsumerID, cli.findServiceName)
	}
}

func TestGetService_ErrorPropagates(t *testing.T) {
	boom := errors.New("discovery failed")
	r := New(&recordingClient{findInstancesErr: boom})

	if _, err := r.GetService(context.Background(), "helloworld"); !errors.Is(err, boom) {
		t.Errorf("GetService() error = %v, want %v", err, boom)
	}
}

// ---------------------------------------------------------------------------
// Register
// ---------------------------------------------------------------------------

func TestRegister_NewService(t *testing.T) {
	cli := &recordingClient{registerServiceResult: "svc-100"}
	r := New(cli)

	svc := &wind.Instance{
		ID:        "inst-1",
		Name:      "helloworld",
		Version:   "1.0.0",
		Endpoints: []string{"grpc://127.0.0.1:9000"},
	}
	if err := r.Register(context.Background(), svc); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if curServiceID != "svc-100" {
		t.Errorf("curServiceID = %q, want svc-100 (set after fresh registration)", curServiceID)
	}

	cli.mu.Lock()
	inst := cli.registeredInstance
	cli.mu.Unlock()
	if inst == nil {
		t.Fatal("RegisterMicroServiceInstance was not called")
	}
	if inst.InstanceId != "inst-1" || inst.HostName != "inst-1" {
		t.Errorf("InstanceId/HostName = %q/%q, want inst-1/inst-1", inst.InstanceId, inst.HostName)
	}
	if inst.ServiceId != "svc-100" {
		t.Errorf("ServiceId = %q, want svc-100", inst.ServiceId)
	}
	if inst.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", inst.Version)
	}
	if len(inst.Endpoints) != 1 || inst.Endpoints[0] != "grpc://127.0.0.1:9000" {
		t.Errorf("Endpoints = %v, want the instance endpoints", inst.Endpoints)
	}
	if inst.Properties[appIDKey] != appID || inst.Properties[envKey] != env {
		t.Errorf("Properties = %v, want appId=%q environment=%q", inst.Properties, appID, env)
	}
}

// Without a preset ID the registry must generate a UUID v4 instance ID.
func TestRegister_GeneratesInstanceID(t *testing.T) {
	cli := &recordingClient{registerServiceResult: "svc-1"}
	r := New(cli)

	svc := &wind.Instance{Name: "helloworld", Version: "1.0.0", Endpoints: []string{"grpc://127.0.0.1:9000"}}
	if err := r.Register(context.Background(), svc); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if svc.ID == "" {
		t.Fatal("Register() left instance ID empty, want a generated UUID")
	}
	if len(svc.ID) != 36 || strings.Count(svc.ID, "-") != 4 {
		t.Errorf("generated ID = %q, want a UUID v4 shape", svc.ID)
	}

	cli.mu.Lock()
	defer cli.mu.Unlock()
	if cli.registeredInstance.InstanceId != svc.ID {
		t.Errorf("registered InstanceId = %q, want %q", cli.registeredInstance.InstanceId, svc.ID)
	}
}

// Registering an already-existing service falls back to GetMicroServiceID.
func TestRegister_ServiceAlreadyExists_UsesExistingID(t *testing.T) {
	cli := &recordingClient{
		registerServiceErr: registryExceptionFor(pb.ErrServiceAlreadyExists),
		getServiceIDResult: "svc-existing",
	}
	r := New(cli)

	svc := &wind.Instance{ID: "inst-1", Name: "helloworld", Version: "1.0.0", Endpoints: []string{"grpc://127.0.0.1:9000"}}
	if err := r.Register(context.Background(), svc); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	cli.mu.Lock()
	defer cli.mu.Unlock()
	if cli.registeredInstance == nil || cli.registeredInstance.ServiceId != "svc-existing" {
		t.Errorf("registered instance = %+v, want it bound to svc-existing", cli.registeredInstance)
	}
	if cli.getServiceIDName != "helloworld" || cli.getServiceIDVer != "1.0.0" {
		t.Errorf("GetMicroServiceID(name=%q, version=%q), want helloworld/1.0.0", cli.getServiceIDName, cli.getServiceIDVer)
	}
}

func TestRegister_ServiceAlreadyExists_GetIDFails(t *testing.T) {
	boom := errors.New("lookup failed")
	cli := &recordingClient{
		registerServiceErr: registryExceptionFor(pb.ErrServiceAlreadyExists),
		getServiceIDErr:    boom,
	}
	r := New(cli)

	if _, err := callRegister(r); !errors.Is(err, boom) {
		t.Errorf("Register() error = %v, want %v", err, boom)
	}
}

// A plain (non-RegistryException) error from RegisterService must abort.
func TestRegister_PlainErrorPropagates(t *testing.T) {
	boom := errors.New("connection refused")
	r := New(&recordingClient{registerServiceErr: boom})

	if _, err := callRegister(r); !errors.Is(err, boom) {
		t.Errorf("Register() error = %v, want %v", err, boom)
	}
}

// A RegistryException whose Message is not JSON must surface the parse error.
func TestRegister_ExceptionWithNonJSONMessage(t *testing.T) {
	cli := &recordingClient{
		registerServiceErr: &sc.RegistryException{Title: "boom", Message: "not json"},
	}
	r := New(cli)

	err := callRegisterErr(t, r)
	if err == nil {
		t.Fatal("Register() should fail on a non-JSON exception message")
	}
	// The parse error must not be the original registry error.
	if errors.Is(err, cli.registerServiceErr) {
		t.Errorf("Register() error = %v, want the JSON parse error instead", err)
	}
}

// A RegistryException with any other error code must propagate unchanged.
func TestRegister_ExceptionWithOtherCode(t *testing.T) {
	orig := registryExceptionFor(500001)
	r := New(&recordingClient{registerServiceErr: orig})

	err := callRegisterErr(t, r)
	if !errors.Is(err, orig) {
		t.Errorf("Register() error = %v, want the original RegistryException", err)
	}
}

func TestRegister_InstanceRegistrationFails(t *testing.T) {
	boom := errors.New("instance rejected")
	cli := &recordingClient{
		registerServiceResult: "svc-1",
		registerInstanceErr:   boom,
	}
	r := New(cli)

	if _, err := callRegister(r); !errors.Is(err, boom) {
		t.Errorf("Register() error = %v, want %v", err, boom)
	}
}

func callRegister(r *Registry) (*wind.Instance, error) {
	svc := &wind.Instance{ID: "inst-1", Name: "helloworld", Version: "1.0.0", Endpoints: []string{"grpc://127.0.0.1:9000"}}
	return svc, r.Register(context.Background(), svc)
}

func callRegisterErr(t *testing.T, r *Registry) error {
	t.Helper()
	svc := &wind.Instance{ID: "inst-1", Name: "helloworld", Version: "1.0.0", Endpoints: []string{"grpc://127.0.0.1:9000"}}
	return r.Register(context.Background(), svc)
}

// ---------------------------------------------------------------------------
// Deregister
// ---------------------------------------------------------------------------

func TestDeregister_UnregistersByServiceAndInstanceID(t *testing.T) {
	cli := &recordingClient{getServiceIDResult: "svc-42"}
	r := New(cli)

	svc := &wind.Instance{ID: "inst-9", Name: "helloworld", Version: "1.0.0"}
	if err := r.Deregister(context.Background(), svc); err != nil {
		t.Fatalf("Deregister() error = %v", err)
	}

	cli.mu.Lock()
	defer cli.mu.Unlock()
	if cli.unregisterSID != "svc-42" || cli.unregisterIID != "inst-9" {
		t.Errorf("UnregisterMicroServiceInstance(%q, %q), want (svc-42, inst-9)", cli.unregisterSID, cli.unregisterIID)
	}
	if cli.getServiceIDEnv != env || cli.getServiceIDName != "helloworld" || cli.getServiceIDVer != "1.0.0" {
		t.Errorf("GetMicroServiceID(app=%q env=%q name=%q ver=%q), want appID/env/helloworld/1.0.0", cli.getServiceIDApp, cli.getServiceIDEnv, cli.getServiceIDName, cli.getServiceIDVer)
	}
}

func TestDeregister_LookupFails(t *testing.T) {
	boom := errors.New("lookup failed")
	cli := &recordingClient{getServiceIDErr: boom}
	r := New(cli)

	if err := r.Deregister(context.Background(), &wind.Instance{ID: "i", Name: "svc", Version: "1"}); !errors.Is(err, boom) {
		t.Errorf("Deregister() error = %v, want %v", err, boom)
	}
	cli.mu.Lock()
	defer cli.mu.Unlock()
	if cli.unregisterSID != "" {
		t.Error("UnregisterMicroServiceInstance was called despite the lookup failure")
	}
}

func TestDeregister_UnregisterFails(t *testing.T) {
	boom := errors.New("unregister failed")
	cli := &recordingClient{
		getServiceIDResult: "svc-42",
		unregisterErr:      boom,
	}
	r := New(cli)

	if err := r.Deregister(context.Background(), &wind.Instance{ID: "i", Name: "svc", Version: "1"}); !errors.Is(err, boom) {
		t.Errorf("Deregister() error = %v, want %v", err, boom)
	}
}

// ---------------------------------------------------------------------------
// Watcher
// ---------------------------------------------------------------------------

func TestWatch_ErrorFromDiscoveryPropagates(t *testing.T) {
	boom := errors.New("discovery failed")
	r := New(&recordingClient{findInstancesErr: boom})

	if _, err := r.Watch(context.Background(), "helloworld"); !errors.Is(err, boom) {
		t.Errorf("Watch() error = %v, want %v", err, boom)
	}
}

func TestWatch_RegistersWithCurrentServiceID(t *testing.T) {
	old := curServiceID
	curServiceID = "consumer-1"
	defer func() { curServiceID = old }()

	cli := &recordingClient{}
	r := New(cli)

	w, err := r.Watch(context.Background(), "helloworld")
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	if w == nil {
		t.Fatal("Watch() returned a nil watcher")
	}

	// The watch goroutine registers the subscription asynchronously; waiting
	// for the callback also guarantees the recorded service ID is visible.
	waitWatchCallback(t, cli)

	cli.mu.Lock()
	defer cli.mu.Unlock()
	if cli.watchServiceID != "consumer-1" {
		t.Errorf("WatchMicroService(id=%q), want consumer-1 (curServiceID)", cli.watchServiceID)
	}
	_ = w.Stop()
}

// Wait until the watch goroutine has registered its callback.
func waitWatchCallback(t *testing.T, cli *recordingClient) func(*sc.MicroServiceInstanceChangedEvent) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		cli.mu.Lock()
		callback := cli.watchCallback
		cli.mu.Unlock()
		if callback != nil {
			return callback
		}
		if time.Now().After(deadline) {
			t.Fatal("WatchMicroService callback was never registered")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Events for other services must be filtered out; matching events are
// converted with their instance fields intact.
func TestWatch_CallbackFiltersAndConverts(t *testing.T) {
	cli := &recordingClient{}
	r := New(cli)

	w, err := r.Watch(context.Background(), "helloworld")
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	sbw := w.(*Watcher)
	defer func() { _ = sbw.Stop() }()

	// WatchMicroService is invoked from a goroutine, so wait until the
	// callback has been registered.
	callback := waitWatchCallback(t, cli)

	// Put sends on an unbuffered channel, so Next must already be waiting
	// before events are fed in.
	type result struct {
		instances []*wind.Instance
	}
	resCh := make(chan result, 1)
	go func() {
		ins, err := sbw.Next(context.Background())
		if err != nil {
			t.Errorf("watcher.Next() error = %v", err)
		}
		resCh <- result{instances: ins}
	}()

	// A different service name must be dropped.
	callback(&sc.MicroServiceInstanceChangedEvent{
		Action: "CREATE",
		Key:    &pb.MicroServiceKey{ServiceName: "other-service", Version: "9.9.9"},
		Instance: &pb.MicroServiceInstance{
			InstanceId: "wrong",
			Endpoints:  []string{"tcp://127.0.0.1:1"},
		},
	})

	// The matching event must be delivered with converted fields.
	callback(&sc.MicroServiceInstanceChangedEvent{
		Action: "CREATE",
		Key:    &pb.MicroServiceKey{ServiceName: "helloworld", Version: "2.0.0"},
		Instance: &pb.MicroServiceInstance{
			InstanceId: "inst-7",
			ServiceId:  "svc-7",
			Properties: map[string]string{"env": "prod"},
			Endpoints:  []string{"grpc://127.0.0.2:9001"},
		},
	})

	var got []*wind.Instance
	select {
	case res := <-resCh:
		got = res.instances
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for watcher.Next()")
	}
	if len(got) != 1 {
		t.Fatalf("watcher.Next() returned %d instances, want 1", len(got))
	}
	inst := got[0]
	if inst.ID != "inst-7" {
		t.Errorf("ID = %q, want inst-7 (the wrong-service event must be filtered)", inst.ID)
	}
	if inst.Name != "helloworld" || inst.Version != "2.0.0" {
		t.Errorf("Name/Version = %q/%q, want helloworld/2.0.0", inst.Name, inst.Version)
	}
	if inst.Metadata["env"] != "prod" {
		t.Errorf("Metadata = %v, want env=prod", inst.Metadata)
	}
	if len(inst.Endpoints) != 1 || inst.Endpoints[0] != "grpc://127.0.0.2:9001" {
		t.Errorf("Endpoints = %v, want [grpc://127.0.0.2:9001]", inst.Endpoints)
	}
}

// An error from WatchMicroService must not panic or block the watcher
// construction (the current implementation silently ignores it).
func TestWatch_CallbackRegistrationErrorIgnored(t *testing.T) {
	r := New(&recordingClient{watchErr: errors.New("watch refused")})

	w, err := r.Watch(context.Background(), "helloworld")
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	if w == nil {
		t.Fatal("Watch() returned nil despite succeeding")
	}
	_ = w.Stop()
}

// ---------------------------------------------------------------------------
// Lifecycle regressions: heartbeat stop wiring and watcher close safety.
// ---------------------------------------------------------------------------

// Register must wire the heartbeat loop to a stop signal and Deregister must
// close it; without the wiring the heartbeat goroutine runs forever.
func TestRegister_Deregister_StopsHeartbeat(t *testing.T) {
	cli := &recordingClient{registerServiceResult: "svc-hb"}
	r := New(cli)

	svc := &wind.Instance{ID: "inst-hb", Name: "helloworld", Version: "1.0.0", Endpoints: []string{"grpc://127.0.0.1:9000"}}
	if err := r.Register(context.Background(), svc); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	r.mu.Lock()
	stop, ok := r.stops["inst-hb"]
	r.mu.Unlock()
	if !ok {
		t.Fatal("Register() did not record a heartbeat stop signal for the instance")
	}

	if err := r.Deregister(context.Background(), svc); err != nil {
		t.Fatalf("Deregister() error = %v", err)
	}

	r.mu.Lock()
	_, still := r.stops["inst-hb"]
	r.mu.Unlock()
	if still {
		t.Error("Deregister() left the heartbeat stop signal registered")
	}

	// The stop signal must be closed so the heartbeat loop exits promptly.
	select {
	case <-stop:
	default:
		t.Error("Deregister() did not close the heartbeat stop signal; the loop would run forever")
	}

	// A second Deregister must not panic (signal already closed and removed).
	if err := r.Deregister(context.Background(), svc); err != nil {
		t.Fatalf("second Deregister() error = %v", err)
	}
}

// Re-registering the same instance ID must replace the stop signal instead of
// leaking a second heartbeat loop or closing the live one.
func TestRegister_Twice_ReplacesHeartbeatStopSignal(t *testing.T) {
	cli := &recordingClient{registerServiceResult: "svc-hb"}
	r := New(cli)

	svc := &wind.Instance{ID: "inst-hb2", Name: "helloworld", Version: "1.0.0", Endpoints: []string{"grpc://127.0.0.1:9000"}}
	if err := r.Register(context.Background(), svc); err != nil {
		t.Fatalf("first Register() error = %v", err)
	}
	r.mu.Lock()
	first := r.stops["inst-hb2"]
	r.mu.Unlock()

	if err := r.Register(context.Background(), svc); err != nil {
		t.Fatalf("second Register() error = %v", err)
	}
	r.mu.Lock()
	second, ok := r.stops["inst-hb2"]
	r.mu.Unlock()
	if !ok || second == first {
		t.Fatal("second Register() did not install a fresh heartbeat stop signal")
	}

	select {
	case <-first:
	default:
		t.Error("the superseded stop signal was not closed; the old heartbeat loop would leak")
	}

	if err := r.Deregister(context.Background(), svc); err != nil {
		t.Fatalf("Deregister() error = %v", err)
	}
}

// After Stop, a late SDK callback must not panic on a closed channel nor
// block, and Next must return promptly instead of blocking forever.
func TestWatcher_StopMakesPutNoopAndNextReturns(t *testing.T) {
	r := New(&recordingClient{})

	w, err := r.Watch(context.Background(), "helloworld")
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	sbw := w.(*Watcher)
	if err := sbw.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	// Stop must be idempotent.
	if err := sbw.Stop(); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}

	// A late SDK callback must not panic nor block.
	putDone := make(chan struct{})
	go func() {
		defer close(putDone)
		sbw.Put(&wind.Instance{ID: "late"})
	}()
	select {
	case <-putDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Put blocked after Stop; sends must become no-ops once the watcher is stopped")
	}

	// Next must return promptly with the watcher's context error.
	if _, err := sbw.Next(context.Background()); !errors.Is(err, context.Canceled) {
		t.Errorf("Next() after Stop error = %v, want context.Canceled", err)
	}
}

// Next must respect its own context even while the watcher is still running.
func TestWatcher_Next_RespectsCallerContext(t *testing.T) {
	r := New(&recordingClient{})

	w, err := r.Watch(context.Background(), "helloworld")
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	sbw := w.(*Watcher)
	defer func() { _ = sbw.Stop() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := sbw.Next(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Next() with a canceled ctx error = %v, want context.Canceled", err)
	}
}
