package eureka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	wind "github.com/tx7do/go-wind"
)

// ---------------------------------------------------------------------------
// Fake Eureka HTTP API — implements the /eureka/v2 endpoints used by Client.
// ---------------------------------------------------------------------------

type RecordedRequest struct {
	Method string
	Path   string
	Query  string
	Body   string
}

type fakeEureka struct {
	mu       sync.Mutex
	server   *httptest.Server
	requests []RecordedRequest

	// registerStatus overrides the status code returned for instance
	// registration POSTs (0 = 204).
	registerStatus int
	// heartbeatStatus overrides the status code returned for heartbeat PUTs
	// (0 = 200).
	heartbeatStatus int
	// apps is the /eureka/v2/apps payload.
	apps ApplicationsRootResponse
	// appInstances maps app ID → bare Application JSON for /eureka/v2/apps/<app>.
	appInstances map[string]Application
}

func newFakeEureka(t *testing.T) *fakeEureka {
	t.Helper()
	f := &fakeEureka{appInstances: make(map[string]Application)}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeEureka) requestsWhere(pred func(RecordedRequest) bool) []RecordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []RecordedRequest
	for _, r := range f.requests {
		if pred(r) {
			out = append(out, r)
		}
	}
	return out
}

func (f *fakeEureka) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	// Extract the app ID from /eureka/v2/apps/<appID> lookups; eureka app IDs
	// are case-insensitive and always stored uppercased.
	appID := ""
	if rest, ok := strings.CutPrefix(r.URL.Path, "/eureka/v2/apps/"); ok && rest != "" {
		appID = strings.ToUpper(strings.Split(rest, "/")[0])
	}

	f.mu.Lock()
	f.requests = append(f.requests, RecordedRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.RawQuery,
		Body:   string(body),
	})
	registerStatus := f.registerStatus
	heartbeatStatus := f.heartbeatStatus
	apps := f.apps
	app, hasApp := f.appInstances[appID]
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/eureka/v2/apps/"):
		if registerStatus != 0 {
			w.WriteHeader(registerStatus)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/eureka/v2/apps/") &&
		strings.Contains(r.URL.Path, "/status"):
		w.WriteHeader(http.StatusOK)

	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/eureka/v2/apps/"):
		if heartbeatStatus != 0 {
			w.WriteHeader(heartbeatStatus)
			return
		}
		w.WriteHeader(http.StatusOK)

	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/eureka/v2/apps/"):
		w.WriteHeader(http.StatusOK)

	case r.Method == http.MethodGet && r.URL.Path == "/eureka/v2/apps":
		_ = json.NewEncoder(w).Encode(apps)

	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/eureka/v2/instances/") && !hasApp:
		// Serve /eureka/v2/instances/<id> by scanning every app for the
		// instance ID; the client expects a bare Instance object.
		wantID := strings.TrimPrefix(r.URL.Path, "/eureka/v2/instances/")
		f.mu.Lock()
		var found *Instance
		for _, application := range f.appInstances {
			for i := range application.Instance {
				if application.Instance[i].InstanceID == wantID {
					found = &application.Instance[i]
				}
			}
		}
		f.mu.Unlock()
		if found == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(*found)

	case r.Method == http.MethodGet && hasApp && strings.Contains(strings.TrimPrefix(r.URL.Path, "/eureka/v2/apps/"), "/"):
		// /eureka/v2/apps/<app>/<instance> returns a bare Instance object.
		wantID := strings.Split(strings.TrimPrefix(r.URL.Path, "/eureka/v2/apps/"), "/")[1]
		var found *Instance
		for i := range app.Instance {
			if app.Instance[i].InstanceID == wantID {
				found = &app.Instance[i]
			}
		}
		if found == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(*found)

	case r.Method == http.MethodGet && hasApp:
		_ = json.NewEncoder(w).Encode(app)

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func clientFor(t *testing.T, f *fakeEureka, opts ...ClientOption) *Client {
	t.Helper()
	return NewClient([]string{f.server.URL}, opts...)
}

// upInstance builds an UP instance whose metadata carries the wind keys the
// Registry conversion helpers expect.
func upInstance(app, id, ip string, port int) Instance {
	return Instance{
		InstanceID: id,
		HostName:   app,
		App:        app,
		IPAddr:     ip,
		VipAddress: app,
		Status:     statusUp,
		Port:       Port{Port: port, Enabled: "true"},
		Metadata: map[string]string{
			"ID":        id,
			"Name":      strings.ToLower(app),
			"Version":   "1.0.0",
			"Endpoints": fmt.Sprintf("http://%s:%d", ip, port),
		},
	}
}

// ---------------------------------------------------------------------------
// Client options
// ---------------------------------------------------------------------------

func TestNewClient_Defaults(t *testing.T) {
	c := NewClient([]string{"http://127.0.0.1:8761"})

	if c.eurekaPath != "eureka/v2" {
		t.Errorf("eurekaPath = %q, want eureka/v2", c.eurekaPath)
	}
	if c.maxRetry != 1 {
		t.Errorf("maxRetry = %d, want 1 (len(urls))", c.maxRetry)
	}
	if c.heartbeatInterval != heartbeatTime {
		t.Errorf("heartbeatInterval = %v, want %v", c.heartbeatInterval, heartbeatTime)
	}
	if c.ctx == nil {
		t.Error("ctx = nil, want context.Background()")
	}
	if c.client == nil {
		t.Error("client = nil, want an initialized HTTP client")
	}
	if len(c.urls) != 1 || c.urls[0] != "http://127.0.0.1:8761" {
		t.Errorf("urls = %v, want the provided server list", c.urls)
	}
}

func TestNewClient_Options(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := NewClient([]string{"http://a", "http://b"},
		WithMaxRetry(5),
		WithHeartbeatInterval(time.Second),
		WithClientContext(ctx),
		WithNamespace("custom/v1"),
	)

	if c.maxRetry != 5 {
		t.Errorf("WithMaxRetry: maxRetry = %d, want 5", c.maxRetry)
	}
	if c.heartbeatInterval != time.Second {
		t.Errorf("WithHeartbeatInterval: got %v, want 1s", c.heartbeatInterval)
	}
	if c.ctx != ctx {
		t.Error("WithClientContext: ctx was not replaced")
	}
	if c.eurekaPath != "custom/v1" {
		t.Errorf("WithNamespace: eurekaPath = %q, want custom/v1", c.eurekaPath)
	}
}

// ---------------------------------------------------------------------------
// URL building and server selection
// ---------------------------------------------------------------------------

func TestPickServer(t *testing.T) {
	c := NewClient([]string{"http://a", "http://b", "http://c"}, WithMaxRetry(3))

	if got := c.pickServer(0); got != "http://a" {
		t.Errorf("pickServer(0) = %q, want http://a", got)
	}
	if got := c.pickServer(1); got != "http://b" {
		t.Errorf("pickServer(1) = %q, want http://b", got)
	}
	if got := c.pickServer(4); got != "http://b" {
		t.Errorf("pickServer(4) = %q, want http://b (wraps modulo len)", got)
	}
}

func TestBuildAPI_JoinsServerPathAndParams(t *testing.T) {
	c := NewClient([]string{"http://127.0.0.1:8761"}, WithNamespace("eureka/v2"))

	got := c.buildAPI(0, "apps", "HELLO")
	want := "http://127.0.0.1:8761/eureka/v2/apps/HELLO"
	if got != want {
		t.Errorf("buildAPI() = %q, want %q", got, want)
	}
}

func TestShuffle_KeepsSameServers(t *testing.T) {
	c := NewClient([]string{"http://a", "http://b", "http://c"})
	before := append([]string(nil), c.urls...)

	c.shuffle()

	if len(c.urls) != 3 {
		t.Fatalf("shuffle changed the URL count to %d", len(c.urls))
	}
	seen := make(map[string]bool)
	for _, u := range c.urls {
		seen[u] = true
	}
	for _, u := range before {
		if !seen[u] {
			t.Errorf("shuffle lost server %q; got %v", u, c.urls)
		}
	}
}

// ---------------------------------------------------------------------------
// Instance registration payload
// ---------------------------------------------------------------------------

func TestClient_Register_BuildsInstancePayload(t *testing.T) {
	f := newFakeEureka(t)
	c := clientFor(t, f)

	ep := Endpoint{
		InstanceID:     "10.0.0.1:HELLO:9000",
		IP:             "10.0.0.1",
		AppID:          "HELLO",
		Port:           9000,
		SecurePort:     443,
		HomePageURL:    "http://10.0.0.1:9000/",
		StatusPageURL:  "http://10.0.0.1:9000/info",
		HealthCheckURL: "http://10.0.0.1:9000/health",
		MetaData:       map[string]string{"env": "test"},
	}
	if err := c.Register(context.Background(), ep); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	regs := f.requestsWhere(func(r RecordedRequest) bool {
		return r.Method == http.MethodPost && r.Path == "/eureka/v2/apps/HELLO"
	})
	if len(regs) != 1 {
		t.Fatalf("server received %d registration POSTs, want 1", len(regs))
	}

	var payload RequestInstance
	if err := json.Unmarshal([]byte(regs[0].Body), &payload); err != nil {
		t.Fatalf("registration body is not valid JSON: %v", err)
	}
	ins := payload.Instance
	if ins.InstanceID != ep.InstanceID {
		t.Errorf("instanceId = %q, want %q", ins.InstanceID, ep.InstanceID)
	}
	if ins.App != "HELLO" {
		t.Errorf("app = %q, want HELLO", ins.App)
	}
	// The client uses the app ID as hostname and VIP address.
	if ins.HostName != "HELLO" || ins.VipAddress != "HELLO" {
		t.Errorf("hostName/vipAddress = %q/%q, want HELLO/HELLO", ins.HostName, ins.VipAddress)
	}
	if ins.IPAddr != "10.0.0.1" {
		t.Errorf("ipAddr = %q, want 10.0.0.1", ins.IPAddr)
	}
	if ins.Status != statusUp {
		t.Errorf("status = %q, want UP", ins.Status)
	}
	if ins.Port.Port != 9000 || ins.Port.Enabled != "true" {
		t.Errorf("port = %+v, want {9000 true}", ins.Port)
	}
	if ins.SecurePort.Port != 443 || ins.SecurePort.Enabled != "false" {
		t.Errorf("securePort = %+v, want {443 false}", ins.SecurePort)
	}
	if ins.HomePageURL != ep.HomePageURL || ins.StatusPageURL != ep.StatusPageURL || ins.HealthCheckURL != ep.HealthCheckURL {
		t.Errorf("urls = %q/%q/%q, want the endpoint URLs", ins.HomePageURL, ins.StatusPageURL, ins.HealthCheckURL)
	}
	if ins.DataCenterInfo.Name != "MyOwn" {
		t.Errorf("dataCenterInfo.name = %q, want MyOwn", ins.DataCenterInfo.Name)
	}
	if ins.Metadata["env"] != "test" {
		t.Errorf("metadata = %v, want env=test carried over", ins.Metadata)
	}
}

func TestClient_Register_ServerError(t *testing.T) {
	f := newFakeEureka(t)
	f.mu.Lock()
	f.registerStatus = http.StatusInternalServerError
	f.mu.Unlock()
	c := clientFor(t, f)

	err := c.Register(context.Background(), Endpoint{AppID: "HELLO", InstanceID: "i1"})
	if err == nil {
		t.Fatal("Register() with a 500 from eureka should fail")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("Register() error = %v, want it to mention status 500", err)
	}
}

// ---------------------------------------------------------------------------
// Deregister / Out / Down / Heartbeat
// ---------------------------------------------------------------------------

func TestClient_Deregister(t *testing.T) {
	f := newFakeEureka(t)
	c := clientFor(t, f)

	// Seed the keepalive channel so cancelHeartbeat finds a running heartbeat.
	c.lock.Lock()
	c.keepalive["HELLO"] = make(chan struct{}, 1)
	c.lock.Unlock()

	if err := c.Deregister(context.Background(), "HELLO", "i1"); err != nil {
		t.Fatalf("Deregister() error = %v", err)
	}

	dels := f.requestsWhere(func(r RecordedRequest) bool {
		return r.Method == http.MethodDelete && r.Path == "/eureka/v2/apps/HELLO/i1"
	})
	if len(dels) != 1 {
		t.Errorf("server received %d DELETEs for HELLO/i1, want 1", len(dels))
	}

	// cancelHeartbeat runs asynchronously and must signal the keepalive
	// channel without blocking.
	deadline := time.Now().Add(5 * time.Second)
	for {
		select {
		case <-c.keepalive["HELLO"]:
			return
		default:
		}
		if time.Now().After(deadline) {
			t.Error("Deregister() did not signal the keepalive channel")
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestClient_OutAndDown_SetStatus(t *testing.T) {
	f := newFakeEureka(t)
	c := clientFor(t, f)

	if err := c.Out(context.Background(), "HELLO", "i1"); err != nil {
		t.Fatalf("Out() error = %v", err)
	}
	if err := c.Down(context.Background(), "HELLO", "i1"); err != nil {
		t.Fatalf("Down() error = %v", err)
	}

	outs := f.requestsWhere(func(r RecordedRequest) bool {
		return r.Method == http.MethodPut && strings.HasSuffix(r.Path, "/status")
	})
	if len(outs) != 2 {
		t.Fatalf("server received %d status PUTs, want 2", len(outs))
	}
	if !strings.HasSuffix(outs[0].Path, "/HELLO/i1/status") || outs[0].Query != "value=OUT_OF_SERVICE" {
		t.Errorf("Out() request = %s?%s, want status?value=OUT_OF_SERVICE", outs[0].Path, outs[0].Query)
	}
	if !strings.HasSuffix(outs[1].Path, "/HELLO/i1/status") || outs[1].Query != "value=DOWN" {
		t.Errorf("Down() request = %s?%s, want status?value=DOWN", outs[1].Path, outs[1].Query)
	}
}

// Heartbeat PUTs periodically and re-registers after heartbeatRetry
// consecutive failures; cancelHeartbeat stops the loop.
func TestClient_Heartbeat_LoopsThenCancels(t *testing.T) {
	f := newFakeEureka(t)
	f.mu.Lock()
	f.heartbeatStatus = http.StatusInternalServerError
	f.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := NewClient([]string{f.server.URL}, WithHeartbeatInterval(20*time.Millisecond), WithClientContext(ctx))

	ep := Endpoint{AppID: "HELLO", InstanceID: "i1", IP: "10.0.0.1", Port: 9000}
	go c.Heartbeat(ep)

	// After heartbeatRetry (3) failures the loop falls back to re-registering.
	deadline := time.Now().Add(5 * time.Second)
	for {
		regs := f.requestsWhere(func(r RecordedRequest) bool {
			return r.Method == http.MethodPost && r.Path == "/eureka/v2/apps/HELLO"
		})
		if len(regs) > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if regs := f.requestsWhere(func(r RecordedRequest) bool {
		return r.Method == http.MethodPost && r.Path == "/eureka/v2/apps/HELLO"
	}); len(regs) == 0 {
		t.Error("Heartbeat() did not re-register after repeated failures")
	}

	// Cancelling must stop the heartbeat loop.
	c.cancelHeartbeat("HELLO")
	time.Sleep(100 * time.Millisecond)
	f.mu.Lock()
	count := len(f.requests)
	f.mu.Unlock()

	time.Sleep(150 * time.Millisecond)
	f.mu.Lock()
	after := len(f.requests)
	f.mu.Unlock()

	if after > count {
		t.Errorf("heartbeat issued %d more requests after cancelHeartbeat, want 0", after-count)
	}
}

// ---------------------------------------------------------------------------
// Discovery queries
// ---------------------------------------------------------------------------

func TestClient_FetchApps(t *testing.T) {
	f := newFakeEureka(t)
	f.mu.Lock()
	f.apps = ApplicationsRootResponse{
		ApplicationsResponse: ApplicationsResponse{
			Applications: []Application{
				{Name: "HELLO", Instance: []Instance{upInstance("HELLO", "i1", "10.0.0.1", 9000)}},
			},
		},
	}
	f.mu.Unlock()
	c := clientFor(t, f)

	apps := c.FetchApps(context.Background())
	if len(apps) != 1 || apps[0].Name != "HELLO" {
		t.Fatalf("FetchApps() = %+v, want one app HELLO", apps)
	}
	if len(apps[0].Instance) != 1 || apps[0].Instance[0].InstanceID != "i1" {
		t.Errorf("FetchApps()[0].Instance = %+v, want one instance i1", apps[0].Instance)
	}

	// The port decodes through the custom "$"/"@enabled" JSON keys.
	if got := apps[0].Instance[0].Port.Port; got != 9000 {
		t.Errorf("instance port = %d, want 9000", got)
	}
}

func TestClient_FetchAllUpInstances_FiltersStatus(t *testing.T) {
	f := newFakeEureka(t)
	down := upInstance("HELLO", "i2", "10.0.0.2", 9001)
	down.Status = statusDown
	f.mu.Lock()
	f.apps = ApplicationsRootResponse{
		ApplicationsResponse: ApplicationsResponse{
			Applications: []Application{
				{Name: "HELLO", Instance: []Instance{
					upInstance("HELLO", "i1", "10.0.0.1", 9000),
					down,
					// A second app with an OUT_OF_SERVICE instance.
				}},
				{Name: "WORLD", Instance: []Instance{func() Instance {
					oos := upInstance("WORLD", "i3", "10.0.0.3", 9002)
					oos.Status = statusOutOfService
					return oos
				}()}},
			},
		},
	}
	f.mu.Unlock()
	c := clientFor(t, f)

	got := c.FetchAllUpInstances(context.Background())
	if len(got) != 1 || got[0].InstanceID != "i1" {
		t.Errorf("FetchAllUpInstances() = %+v, want only the UP instance i1", got)
	}
}

func TestClient_FetchAppInstances_And_UpInstances(t *testing.T) {
	f := newFakeEureka(t)
	f.mu.Lock()
	f.appInstances["HELLO"] = Application{
		Name: "HELLO",
		Instance: []Instance{
			upInstance("HELLO", "i1", "10.0.0.1", 9000),
			upInstance("HELLO", "i2", "10.0.0.2", 9001),
		},
	}
	f.mu.Unlock()
	c := clientFor(t, f)

	app, err := c.FetchAppInstances(context.Background(), "HELLO")
	if err != nil {
		t.Fatalf("FetchAppInstances() error = %v", err)
	}
	if app.Name != "HELLO" || len(app.Instance) != 2 {
		t.Errorf("FetchAppInstances() = %+v, want HELLO with 2 instances", app)
	}

	ups := c.FetchAppUpInstances(context.Background(), "HELLO")
	if len(ups) != 2 {
		t.Errorf("FetchAppUpInstances() = %d instances, want 2 (all UP)", len(ups))
	}
}

func TestClient_FetchAppInstances_Error(t *testing.T) {
	f := newFakeEureka(t) // no app seeded → 404
	c := clientFor(t, f)

	if _, err := c.FetchAppInstances(context.Background(), "HELLO"); err == nil {
		t.Error("FetchAppInstances() for a missing app should fail")
	}
	if got := c.FetchAppUpInstances(context.Background(), "HELLO"); got != nil {
		t.Errorf("FetchAppUpInstances() on error = %v, want nil", got)
	}
}

func TestClient_FetchAppInstance_And_FetchInstance(t *testing.T) {
	f := newFakeEureka(t)
	f.mu.Lock()
	f.appInstances["HELLO"] = Application{
		Name:     "HELLO",
		Instance: []Instance{upInstance("HELLO", "i1", "10.0.0.1", 9000)},
	}
	f.mu.Unlock()
	c := clientFor(t, f)

	ins, err := c.FetchAppInstance(context.Background(), "HELLO", "i1")
	if err != nil {
		t.Fatalf("FetchAppInstance() error = %v", err)
	}
	if ins.InstanceID != "i1" || ins.Port.Port != 9000 {
		t.Errorf("FetchAppInstance() = %+v, want instance i1 on port 9000", ins)
	}

	// /eureka/v2/instances/<id> is resolved by scanning the seeded apps.
	byID, err := c.FetchInstance(context.Background(), "i1")
	if err != nil {
		t.Fatalf("FetchInstance() error = %v", err)
	}
	if byID.InstanceID != "i1" {
		t.Errorf("FetchInstance() = %+v, want instance i1", byID)
	}

	if _, err := c.FetchInstance(context.Background(), "missing"); err == nil {
		t.Error("FetchInstance() for a missing instance should fail")
	}
}

// ---------------------------------------------------------------------------
// Retry / failover behaviour
// ---------------------------------------------------------------------------

// A request that fails against one server must be retried against the next
// URL in the list.
func TestClient_Do_FailsOverAcrossServers(t *testing.T) {
	f := newFakeEureka(t)
	// 127.0.0.1:1 is a closed local port; the connection is refused
	// immediately without touching the network.
	c := NewClient([]string{"http://127.0.0.1:1", f.server.URL}, WithMaxRetry(2))

	if err := c.Register(context.Background(), Endpoint{AppID: "HELLO", InstanceID: "i1"}); err != nil {
		t.Fatalf("Register() with one dead server should fail over, got error: %v", err)
	}
	if regs := f.requestsWhere(func(r RecordedRequest) bool {
		return r.Method == http.MethodPost
	}); len(regs) != 1 {
		t.Errorf("fake server received %d POSTs, want 1 after failover", len(regs))
	}
}

// Exhausting every server yields the retry-exceeded error.
func TestClient_Do_AllServersDown(t *testing.T) {
	c := NewClient([]string{"http://127.0.0.1:1", "http://127.0.0.2:1"}, WithMaxRetry(2))

	err := c.Register(context.Background(), Endpoint{AppID: "HELLO", InstanceID: "i1"})
	if err == nil {
		t.Fatal("Register() with only dead servers should fail")
	}
	if !strings.Contains(err.Error(), "retry after 2 times") {
		t.Errorf("Register() error = %v, want retry-exhausted message", err)
	}
}

// ---------------------------------------------------------------------------
// filterUp
// ---------------------------------------------------------------------------

func TestFilterUp(t *testing.T) {
	c := NewClient([]string{"http://127.0.0.1:1"})
	apps := []Application{
		{Instance: []Instance{
			{InstanceID: "up", Status: statusUp},
			{InstanceID: "down", Status: statusDown},
		}},
		{Instance: []Instance{
			{InstanceID: "oos", Status: statusOutOfService},
			{InstanceID: "up2", Status: statusUp},
		}},
	}

	got := c.filterUp(apps...)
	ids := make([]string, 0, len(got))
	for _, ins := range got {
		ids = append(ids, ins.InstanceID)
	}
	if len(ids) != 2 || ids[0] != "up" || ids[1] != "up2" {
		t.Errorf("filterUp() = %v, want [up up2]", ids)
	}
}

// ---------------------------------------------------------------------------
// API — app-ID mapping, caching, subscription
// ---------------------------------------------------------------------------

func newTestAPI(t *testing.T, f *fakeEureka, apps Application, refresh time.Duration) *API {
	t.Helper()
	f.mu.Lock()
	f.apps = ApplicationsRootResponse{
		ApplicationsResponse: ApplicationsResponse{
			Applications: []Application{apps},
		},
	}
	f.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c := NewClient([]string{f.server.URL}, WithClientContext(ctx))
	return NewAPI(ctx, c, refresh)
}

func TestAPI_ToAppID(t *testing.T) {
	f := newFakeEureka(t)
	api := newTestAPI(t, f, Application{}, time.Hour)

	if got := api.ToAppID("helloworld"); got != "HELLOWORLD" {
		t.Errorf("ToAppID(helloworld) = %q, want HELLOWORLD", got)
	}
	if got := api.ToAppID("ALREADY-UP"); got != "ALREADY-UP" {
		t.Errorf("ToAppID(ALREADY-UP) = %q, want unchanged", got)
	}
}

func TestAPI_GetService_FromCache(t *testing.T) {
	f := newFakeEureka(t)
	api := newTestAPI(t, f, Application{
		Name:     "HELLO",
		Instance: []Instance{upInstance("HELLO", "i1", "10.0.0.1", 9000)},
	}, time.Hour)

	// NewAPI broadcasts once at startup; wait for the cache to fill.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if ins := api.GetService(context.Background(), "hello"); len(ins) > 0 {
			if ins[0].InstanceID != "i1" {
				t.Errorf("GetService()[0] = %+v, want instance i1", ins[0])
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("GetService() never saw the cached instances")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAPI_Subscribe_And_Unsubscribe(t *testing.T) {
	f := newFakeEureka(t)
	api := newTestAPI(t, f, Application{
		Name:     "HELLO",
		Instance: []Instance{upInstance("HELLO", "i1", "10.0.0.1", 9000)},
	}, time.Hour)

	calls := make(chan struct{}, 8)
	if err := api.Subscribe("hello", func() { calls <- struct{}{} }); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	// Subscribe triggers a broadcast, which must invoke the callback.
	select {
	case <-calls:
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe() callback was not invoked")
	}

	api.Unsubscribe("hello")
	api.lock.Lock()
	_, still := api.subscribers["HELLO"]
	api.lock.Unlock()
	if still {
		t.Error("Unsubscribe() did not remove the subscriber")
	}
}

// Registering an endpoint that is already UP must be skipped; a new one is
// posted and heartbeated.
func TestAPI_Register_DeduplicatesExistingInstances(t *testing.T) {
	f := newFakeEureka(t)
	api := newTestAPI(t, f, Application{
		Name:     "HELLO",
		Instance: []Instance{upInstance("HELLO", "i1", "10.0.0.1", 9000)},
	}, time.Hour)

	// Wait for the initial broadcast so the dedup cache is warm.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if len(api.GetService(context.Background(), "HELLO")) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("initial broadcast never populated the cache")
		}
		time.Sleep(10 * time.Millisecond)
	}
	before := len(f.requestsWhere(func(r RecordedRequest) bool { return r.Method == http.MethodPost }))

	// i1 already UP → skipped; i2 is new → POSTed.
	err := api.Register(context.Background(), "hello",
		Endpoint{AppID: "HELLO", InstanceID: "i1", IP: "10.0.0.1", Port: 9000},
		Endpoint{AppID: "HELLO", InstanceID: "i2", IP: "10.0.0.2", Port: 9001},
	)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	posts := f.requestsWhere(func(r RecordedRequest) bool {
		return r.Method == http.MethodPost && strings.HasSuffix(r.Path, "/apps/HELLO")
	})
	if len(posts) != before+1 {
		t.Errorf("server received %d POSTs (had %d), want exactly 1 new for i2", len(posts), before)
	}
}

// ---------------------------------------------------------------------------
// Registry — options, endpoint building, register/deregister/get/watch
// ---------------------------------------------------------------------------

func TestRegistry_New_Defaults(t *testing.T) {
	r, err := New([]string{"http://127.0.0.1:8761"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if r.heartbeatInterval != heartbeatTime {
		t.Errorf("heartbeatInterval = %v, want %v", r.heartbeatInterval, heartbeatTime)
	}
	if r.refreshInterval != refreshTime {
		t.Errorf("refreshInterval = %v, want %v", r.refreshInterval, refreshTime)
	}
	if r.eurekaPath != "eureka/v2" {
		t.Errorf("eurekaPath = %q, want eureka/v2", r.eurekaPath)
	}
	if r.maxRetry != 1 {
		t.Errorf("maxRetry = %d, want 1 (len(urls))", r.maxRetry)
	}
	if r.api == nil {
		t.Error("api = nil, want an initialized API")
	}
}

func TestRegistry_New_Options(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r, err := New([]string{"http://a", "http://b"},
		WithContext(ctx),
		WithHeartbeat(time.Second),
		WithRefresh(2*time.Second),
		WithEurekaPath("custom/v1"),
		MaxRetry(7),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if r.ctx != ctx {
		t.Error("WithContext: ctx was not applied")
	}
	if r.heartbeatInterval != time.Second {
		t.Errorf("WithHeartbeat: got %v, want 1s", r.heartbeatInterval)
	}
	if r.refreshInterval != 2*time.Second {
		t.Errorf("WithRefresh: got %v, want 2s", r.refreshInterval)
	}
	if r.eurekaPath != "custom/v1" {
		t.Errorf("WithEurekaPath: got %q, want custom/v1", r.eurekaPath)
	}
	if r.maxRetry != 7 {
		t.Errorf("MaxRetry: got %d, want 7", r.maxRetry)
	}
}

func TestRegistry_Endpoints_BuildsFromWindInstance(t *testing.T) {
	r, err := New([]string{"http://127.0.0.1:8761"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	svc := &wind.Instance{
		ID:        "0",
		Name:      "helloworld",
		Version:   "1.2.3",
		Endpoints: []string{"http://127.0.0.1:1111"},
		Metadata: map[string]string{
			"securePort":     "8443",
			"homePageURL":    "http://custom/",
			"statusPageURL":  "http://custom/info",
			"healthCheckURL": "http://custom/health",
			"env":            "test",
		},
	}

	eps := r.Endpoints(svc)
	if len(eps) != 1 {
		t.Fatalf("Endpoints() returned %d endpoints, want 1", len(eps))
	}
	ep := eps[0]
	if ep.AppID != "HELLOWORLD" {
		t.Errorf("AppID = %q, want HELLOWORLD", ep.AppID)
	}
	if ep.IP != "127.0.0.1" {
		t.Errorf("IP = %q, want 127.0.0.1", ep.IP)
	}
	if ep.Port != 1111 {
		t.Errorf("Port = %d, want 1111", ep.Port)
	}
	if ep.SecurePort != 8443 {
		t.Errorf("SecurePort = %d, want 8443 (metadata override)", ep.SecurePort)
	}
	if ep.HomePageURL != "http://custom/" || ep.StatusPageURL != "http://custom/info" || ep.HealthCheckURL != "http://custom/health" {
		t.Errorf("URLs = %q/%q/%q, want the metadata overrides", ep.HomePageURL, ep.StatusPageURL, ep.HealthCheckURL)
	}
	wantID := "127.0.0.1:HELLOWORLD:1111"
	if ep.InstanceID != wantID {
		t.Errorf("InstanceID = %q, want %q", ep.InstanceID, wantID)
	}
	if ep.MetaData["ID"] != "0" || ep.MetaData["Name"] != "helloworld" || ep.MetaData["Version"] != "1.2.3" {
		t.Errorf("MetaData identity keys = %v, want ID/Name/Version enriched", ep.MetaData)
	}
	if ep.MetaData["Endpoints"] != "http://127.0.0.1:1111" {
		t.Errorf("MetaData[Endpoints] = %q, want the original endpoint", ep.MetaData["Endpoints"])
	}
	if ep.MetaData["agent"] != "go-eureka-client" {
		t.Errorf("MetaData[agent] = %q, want go-eureka-client", ep.MetaData["agent"])
	}
	if ep.MetaData["env"] != "test" {
		t.Errorf("MetaData[env] = %q, want test (user metadata preserved)", ep.MetaData["env"])
	}
}

func TestRegistry_Endpoints_DefaultsWithoutMetadata(t *testing.T) {
	r, err := New([]string{"http://127.0.0.1:8761"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	svc := &wind.Instance{
		Name:      "demo",
		Endpoints: []string{"grpc://10.1.2.3:9000"},
	}

	eps := r.Endpoints(svc)
	if len(eps) != 1 {
		t.Fatalf("Endpoints() returned %d endpoints, want 1", len(eps))
	}
	ep := eps[0]
	if ep.SecurePort != 443 {
		t.Errorf("SecurePort = %d, want the 443 default", ep.SecurePort)
	}
	if ep.HomePageURL != "grpc://10.1.2.3:9000/" {
		t.Errorf("HomePageURL = %q, want endpoint plus trailing slash", ep.HomePageURL)
	}
	if ep.StatusPageURL != "grpc://10.1.2.3:9000/info" || ep.HealthCheckURL != "grpc://10.1.2.3:9000/health" {
		t.Errorf("StatusPageURL/HealthCheckURL = %q/%q, want endpoint-derived defaults",
			ep.StatusPageURL, ep.HealthCheckURL)
	}
	if ep.MetaData == nil || len(ep.MetaData) == 0 {
		t.Error("MetaData = nil, want an enriched metadata map")
	}
}

// The instance cache must key entries by the normalized (uppercased) app ID on
// both the write and the read side. With mixed keys, appending a second
// instance of the same app resets the slice (append on the nil raw key), so
// earlier instances are silently dropped from the cache.
func TestAPI_CacheAllInstances_NormalizesAppIDKeys(t *testing.T) {
	f := newFakeEureka(t)
	lower := func(id string) Instance {
		ins := upInstance("HELLO", id, "10.0.0.1", 9000)
		ins.App = "hello" // lowercase App, as some eureka servers report
		return ins
	}
	api := newTestAPI(t, f, Application{
		Name:     "HELLO",
		Instance: []Instance{lower("i1"), lower("i2")},
	}, time.Hour)

	deadline := time.Now().Add(5 * time.Second)
	for {
		if ins := api.GetService(context.Background(), "hello"); len(ins) > 0 {
			if len(ins) != 2 {
				t.Fatalf("GetService() returned %d cached instances (%v), want both i1 and i2", len(ins), ins)
			}
			if ins[0].InstanceID != "i1" || ins[1].InstanceID != "i2" {
				t.Errorf("GetService() = %v, want [i1 i2]", ins)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("GetService never saw the cached instances; app-ID key normalization is broken")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// GetService reads allInstances while the background refresh writes it; the
// read must be synchronized (run with -race to verify).
func TestAPI_GetService_ConcurrentWithRefresh(t *testing.T) {
	f := newFakeEureka(t)
	api := newTestAPI(t, f, Application{
		Name:     "HELLO",
		Instance: []Instance{upInstance("HELLO", "i1", "10.0.0.1", 9000)},
	}, 5*time.Millisecond)

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				_ = api.GetService(context.Background(), "hello")
			}
		}
	}()
	time.Sleep(200 * time.Millisecond)
	close(stop)
	<-done
}

// cancelHeartbeat must not block when the heartbeat goroutine has already
// exited and nobody receives on the stored channel.
func TestClient_CancelHeartbeat_DoesNotBlockAfterGoroutineExit(t *testing.T) {
	c := NewClient([]string{"http://127.0.0.1:1"})
	c.lock.Lock()
	c.keepalive["HELLO"] = make(chan struct{}) // unbuffered, no receiver
	c.lock.Unlock()

	done := make(chan struct{})
	go func() {
		c.cancelHeartbeat("HELLO")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelHeartbeat blocked forever on an unbuffered channel with no receiver")
	}
}

// ---------------------------------------------------------------------------
// Registry — endpoint shape validation
// ---------------------------------------------------------------------------

// A portless endpoint like http://host must be skipped instead of panicking
// on ep[start+2:end] with start+2 > end.
func TestRegistry_Endpoints_SkipsPortlessEndpoints(t *testing.T) {
	r, err := New([]string{"http://127.0.0.1:8761"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	svc := &wind.Instance{
		Name: "demo",
		Endpoints: []string{
			"http://host",          // no port → start+2 > end, used to panic
			"localhost:8080",       // no scheme → unparseable, must be skipped
			"http://10.0.0.1:9000", // valid
		},
	}

	var eps []Endpoint
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("Endpoints() panicked on a portless endpoint: %v", rec)
			}
		}()
		eps = r.Endpoints(svc)
	}()

	if len(eps) != 1 {
		t.Fatalf("Endpoints() returned %d endpoints (%+v), want only the valid one", len(eps), eps)
	}
	if eps[0].IP != "10.0.0.1" || eps[0].Port != 9000 {
		t.Errorf("Endpoints()[0] = %+v, want IP 10.0.0.1 port 9000", eps[0])
	}
}

func TestRegistry_Register_Deregister_AgainstFakeServer(t *testing.T) {
	f := newFakeEureka(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r, err := New([]string{f.server.URL},
		WithContext(ctx),
		WithHeartbeat(time.Hour), // no heartbeat ticks during the test
		WithRefresh(time.Hour),   // no background refresh during the test
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	svc := &wind.Instance{
		ID:        "0",
		Name:      "helloworld",
		Version:   "1.0.0",
		Endpoints: []string{"http://127.0.0.1:1111"},
	}

	if err := r.Register(context.Background(), svc); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	posts := f.requestsWhere(func(r RecordedRequest) bool {
		return r.Method == http.MethodPost && r.Path == "/eureka/v2/apps/HELLOWORLD"
	})
	if len(posts) != 1 {
		t.Fatalf("server received %d registration POSTs, want 1", len(posts))
	}

	if err := r.Deregister(context.Background(), svc); err != nil {
		t.Fatalf("Deregister() error = %v", err)
	}
	dels := f.requestsWhere(func(r RecordedRequest) bool {
		return r.Method == http.MethodDelete && r.Path == "/eureka/v2/apps/HELLOWORLD/127.0.0.1:HELLOWORLD:1111"
	})
	if len(dels) != 1 {
		t.Errorf("server received %d DELETEs for the instance, want 1", len(dels))
	}
}

func TestRegistry_GetService_ConvertsMetadata(t *testing.T) {
	f := newFakeEureka(t)
	f.mu.Lock()
	f.apps = ApplicationsRootResponse{
		ApplicationsResponse: ApplicationsResponse{
			Applications: []Application{{
				Name:     "HELLOWORLD",
				Instance: []Instance{upInstance("HELLOWORLD", "i1", "10.0.0.1", 9000)},
			}},
		},
	}
	f.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r, err := New([]string{f.server.URL},
		WithContext(ctx),
		WithHeartbeat(time.Hour),
		WithRefresh(30*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Wait for the background broadcast to fill the cache from the server.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if len(r.api.GetService(ctx, "helloworld")) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("broadcast never populated the instance cache")
		}
		time.Sleep(10 * time.Millisecond)
	}

	got, err := r.GetService(ctx, "helloworld")
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("GetService() returned %d instances, want 1", len(got))
	}
	inst := got[0]
	if inst.ID != "i1" {
		t.Errorf("ID = %q, want i1 (from metadata ID)", inst.ID)
	}
	if inst.Name != "helloworld" {
		t.Errorf("Name = %q, want helloworld (from metadata Name)", inst.Name)
	}
	if inst.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0 (from metadata Version)", inst.Version)
	}
	if len(inst.Endpoints) != 1 || inst.Endpoints[0] != "http://10.0.0.1:9000" {
		t.Errorf("Endpoints = %v, want [http://10.0.0.1:9000]", inst.Endpoints)
	}
}

// ---------------------------------------------------------------------------
// Watcher
// ---------------------------------------------------------------------------

func TestRegistry_Watch_ReceivesUpdates(t *testing.T) {
	f := newFakeEureka(t)
	// Seed the fake with the app the watcher subscribes to; the API's
	// background broadcast then populates the cache from the server.
	f.mu.Lock()
	f.apps = ApplicationsRootResponse{
		ApplicationsResponse: ApplicationsResponse{
			Applications: []Application{{
				Name:     "HELLOWORLD",
				Instance: []Instance{upInstance("HELLOWORLD", "i1", "10.0.0.1", 9000)},
			}},
		},
	}
	f.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r, err := New([]string{f.server.URL},
		WithContext(ctx),
		WithHeartbeat(time.Hour),
		WithRefresh(30*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Wait for the periodic refresh broadcast to fill the cache.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if len(r.api.GetService(ctx, "helloworld")) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("refresh broadcast never populated the instance cache")
		}
		time.Sleep(10 * time.Millisecond)
	}

	w, err := r.Watch(ctx, "helloworld")
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	got, err := w.Next(context.Background())
	if err != nil {
		t.Fatalf("watcher.Next() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "i1" {
		t.Errorf("watcher.Next() = %+v, want instance i1", got)
	}

	if err := w.Stop(); err != nil {
		t.Errorf("watcher.Stop() error = %v", err)
	}
	r.api.lock.Lock()
	_, still := r.api.subscribers["HELLOWORLD"]
	r.api.lock.Unlock()
	if still {
		t.Error("watcher.Stop() did not unsubscribe")
	}
}

// After Stop (context cancellation) and with no pending signal, Next must
// return the context error rather than block.
func TestWatcher_Next_ContextCancelled(t *testing.T) {
	f := newFakeEureka(t)
	api := newTestAPI(t, f, Application{}, time.Hour)

	w := &watcher{
		cli:        api,
		serverName: "helloworld",
		watchChan:  make(chan struct{}, 1),
	}
	w.ctx, w.cancel = context.WithCancel(context.Background())
	w.cancel()

	if _, err := w.Next(context.Background()); !errors.Is(err, context.Canceled) {
		t.Errorf("watcher.Next() after cancellation error = %v, want context.Canceled", err)
	}
}

// ---------------------------------------------------------------------------
// Guards against regressions in small helpers.
// ---------------------------------------------------------------------------

func TestPortUnmarshal_CustomJSONKeys(t *testing.T) {
	raw := `{"port":{"$":8080,"@enabled":"true"},"securePort":{"$":8443,"@enabled":"false"}}`
	var ins Instance
	if err := json.Unmarshal([]byte(raw), &ins); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	if ins.Port.Port != 8080 || ins.Port.Enabled != "true" {
		t.Errorf("port = %+v, want {8080 true}", ins.Port)
	}
	if ins.SecurePort.Port != 8443 || ins.SecurePort.Enabled != "false" {
		t.Errorf("securePort = %+v, want {8443 false}", ins.SecurePort)
	}
}

func TestConstants(t *testing.T) {
	if statusUp != "UP" || statusDown != "DOWN" || statusOutOfService != "OUT_OF_SERVICE" {
		t.Errorf("status constants changed: %q/%q/%q", statusUp, statusDown, statusOutOfService)
	}
	if heartbeatTime != 10*time.Second || refreshTime != 30*time.Second || httpTimeout != 3*time.Second {
		t.Errorf("timing constants changed: %v/%v/%v", heartbeatTime, refreshTime, httpTimeout)
	}
	if heartbeatRetry != 3 {
		t.Errorf("heartbeatRetry = %d, want 3", heartbeatRetry)
	}
	if maxIdleConns != 100 {
		t.Errorf("maxIdleConns = %d, want 100", maxIdleConns)
	}
}
