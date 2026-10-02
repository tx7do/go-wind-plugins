package dtm

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/dtm-labs/client/dtmcli"
)

// ---------------------------------------------------------------------------
// emulated DTM server
// ---------------------------------------------------------------------------

// mockDtm is a minimal hermetic stand-in for the DTM HTTP API: every request
// is recorded and answered with "SUCCESS", except for paths listed in the
// failure set which answer 409 "FAILURE" (the two signals dtmcli inspects).
type mockDtm struct {
	mu      sync.Mutex
	paths   []string
	bodies  []string
	failing map[string]bool
}

func (m *mockDtm) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		m.mu.Lock()
		m.paths = append(m.paths, r.URL.Path)
		m.bodies = append(m.bodies, string(body))
		fail := m.failing[r.URL.Path]
		m.mu.Unlock()

		if fail {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(dtmcli.ResultFailure))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(dtmcli.ResultSuccess))
	}
}

func (m *mockDtm) seen() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.paths...)
}

func (m *mockDtm) seenPaths() map[string]bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]bool{}
	for _, p := range m.paths {
		out[p] = true
	}
	return out
}

func (m *mockDtm) lastBodyFor(path string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.paths) - 1; i >= 0; i-- {
		if m.paths[i] == path {
			return m.bodies[i]
		}
	}
	return ""
}

func newMockDtm(t *testing.T, failing ...string) (server string, m *mockDtm) {
	t.Helper()
	m = &mockDtm{failing: map[string]bool{}}
	for _, p := range failing {
		m.failing[p] = true
	}
	srv := httptest.NewServer(m.handler())
	t.Cleanup(srv.Close)
	return srv.URL, m
}

func containsPath(paths []string, suffix string) bool {
	for _, p := range paths {
		if strings.HasSuffix(p, suffix) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// pure builder additions
// ---------------------------------------------------------------------------

func TestSaga_AddBranchOrder_Chaining(t *testing.T) {
	c := NewClient()
	s := c.NewSaga("gid-order")

	ret := s.AddBranchOrder(1, []int{0})
	if ret != s {
		t.Error("AddBranchOrder must return the same *Saga for chaining")
	}

	ret = s.SetConcurrent().AddBranchOrder(2, []int{0, 1})
	if ret != s {
		t.Error("SetConcurrent/AddBranchOrder must return the same *Saga")
	}
}

func TestNewMsg_UsesClientServer(t *testing.T) {
	c := NewClient(WithServer("http://dtm-mock:36789/api/dtmsvr"))
	m := c.NewMsg("gid")
	if m.msg.Dtm != "http://dtm-mock:36789/api/dtmsvr" {
		t.Errorf("msg Dtm = %q, want the client's configured server", m.msg.Dtm)
	}
	if m.msg.TransType != "msg" {
		t.Errorf("TransType = %q, want msg", m.msg.TransType)
	}
}

// ---------------------------------------------------------------------------
// Saga.Submit against the emulated server
// ---------------------------------------------------------------------------

func TestSaga_Submit_MockServer(t *testing.T) {
	server, md := newMockDtm(t)
	c := NewClient(WithServer(server))

	s := c.NewSaga("saga-gid-1").
		Add(server+"/out", server+"/out/compensate", map[string]string{"k": "v"})

	if err := s.Submit(); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	paths := md.seen()
	if !containsPath(paths, "/submit") {
		t.Errorf("expected a /submit call, got %v", paths)
	}
	body := md.lastBodyFor("/submit")
	if body == "" || !strings.Contains(body, "saga-gid-1") {
		t.Errorf("submit body = %q, want it to contain the gid", body)
	}
	if !strings.Contains(body, `trans_type":"saga`) && !strings.Contains(body, `"trans_type":"saga"`) {
		t.Errorf("submit body = %q, want trans_type saga", body)
	}
}

func TestSaga_Submit_ServerFailure(t *testing.T) {
	server, _ := newMockDtm(t, "/submit")
	c := NewClient(WithServer(server))

	s := c.NewSaga("saga-gid-2").Add("/a", "/a/compensate", "x")
	if err := s.Submit(); err == nil {
		t.Fatal("Submit() expected an error when the server reports FAILURE")
	}
}

// ---------------------------------------------------------------------------
// Msg.Prepare / Submit against the emulated server
// ---------------------------------------------------------------------------

func TestMsg_PrepareAndSubmit_MockServer(t *testing.T) {
	server, md := newMockDtm(t)
	c := NewClient(WithServer(server))

	m := c.NewMsg("msg-gid-1").Add("/api/action", map[string]int{"n": 1})

	if err := m.Prepare("/api/query-prepared"); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if !containsPath(md.seen(), "/prepare") {
		t.Fatalf("expected a /prepare call, got %v", md.seen())
	}
	if body := md.lastBodyFor("/prepare"); !strings.Contains(body, "/api/query-prepared") {
		t.Errorf("prepare body = %q, want it to contain the queryPrepared URL", body)
	}

	if err := m.Submit(); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if !containsPath(md.seen(), "/submit") {
		t.Errorf("expected a /submit call, got %v", md.seen())
	}
}

func TestMsg_Prepare_ServerFailure(t *testing.T) {
	server, _ := newMockDtm(t, "/prepare")
	c := NewClient(WithServer(server))

	m := c.NewMsg("msg-gid-2").Add("/a", "x")
	if err := m.Prepare("/q"); err == nil {
		t.Fatal("Prepare() expected an error when the server reports FAILURE")
	}
}

// ---------------------------------------------------------------------------
// Msg.DoAndSubmit against the emulated server
// ---------------------------------------------------------------------------

func TestMsg_DoAndSubmit_Success(t *testing.T) {
	server, md := newMockDtm(t)
	c := NewClient(WithServer(server))

	var bb *dtmcli.BranchBarrier
	m := c.NewMsg("msg-gid-3").Add("/api/action", "payload")

	err := m.DoAndSubmit("/api/query-prepared", func(b *dtmcli.BranchBarrier) error {
		bb = b
		return nil
	})
	if err != nil {
		t.Fatalf("DoAndSubmit() error = %v", err)
	}

	// The barrier handed to the business function identifies the msg branch.
	if bb == nil {
		t.Fatal("busiCall never received a barrier")
	}
	if bb.TransType != "msg" || bb.Gid != "msg-gid-3" {
		t.Errorf("barrier = %+v, want trans_type msg gid msg-gid-3", bb)
	}
	if bb.BranchID == "" || bb.Op == "" {
		t.Errorf("barrier = %+v, want non-empty BranchID and Op", bb)
	}

	paths := md.seenPaths()
	if !paths["/prepare"] || !paths["/submit"] {
		t.Errorf("expected /prepare and /submit calls, got %v", md.seen())
	}
}

func TestMsg_DoAndSubmit_BusinessFailureAborts(t *testing.T) {
	server, md := newMockDtm(t)
	c := NewClient(WithServer(server))

	m := c.NewMsg("msg-gid-4").Add("/api/action", "payload")

	err := m.DoAndSubmit("/api/query-prepared", func(*dtmcli.BranchBarrier) error {
		return dtmcli.ErrFailure
	})
	if !errors.Is(err, dtmcli.ErrFailure) {
		t.Fatalf("DoAndSubmit() error = %v, want dtmcli.ErrFailure", err)
	}

	paths := md.seenPaths()
	if !paths["/prepare"] || !paths["/abort"] {
		t.Errorf("expected /prepare and /abort calls, got %v", md.seen())
	}
	if paths["/submit"] {
		t.Error("a failed business call must not submit the transaction")
	}
}

// ---------------------------------------------------------------------------
// TCC against the emulated server
// ---------------------------------------------------------------------------

func TestTccGlobalTransaction_Success(t *testing.T) {
	server, md := newMockDtm(t)
	c := NewClient(WithServer(server))

	tryURL := server + "/api/try"
	confirmURL := server + "/api/confirm"
	cancelURL := server + "/api/cancel"

	err := c.TccGlobalTransaction("tcc-gid-1", func(t *Tcc) error {
		return t.CallBranch(map[string]string{"k": "v"}, tryURL, confirmURL, cancelURL)
	})
	if err != nil {
		t.Fatalf("TccGlobalTransaction() error = %v", err)
	}

	paths := md.seenPaths()
	for _, want := range []string{"/prepare", "/registerBranch", "/api/try", "/submit"} {
		if !paths[want] {
			t.Errorf("expected %s call, got %v", want, md.seen())
		}
	}

	// The registerBranch body must carry the confirm/cancel branch URLs.
	body := md.lastBodyFor("/registerBranch")
	if !strings.Contains(body, confirmURL) || !strings.Contains(body, cancelURL) {
		t.Errorf("registerBranch body = %q, want it to contain confirm/cancel URLs", body)
	}
}

func TestTccGlobalTransaction_BranchErrorAborts(t *testing.T) {
	// The branch's try endpoint fails: the global transaction must abort.
	server, md := newMockDtm(t, "/api/try")
	c := NewClient(WithServer(server))

	err := c.TccGlobalTransaction("tcc-gid-2", func(t *Tcc) error {
		return t.CallBranch("body", server+"/api/try", server+"/api/confirm", server+"/api/cancel")
	})
	if err == nil {
		t.Fatal("TccGlobalTransaction() expected an error when the try branch fails")
	}

	paths := md.seenPaths()
	if !paths["/abort"] {
		t.Errorf("expected an /abort call, got %v", md.seen())
	}
	if paths["/submit"] {
		t.Error("a failed TCC branch must not submit the transaction")
	}
}

func TestTcc_CallBranch_RegisterError(t *testing.T) {
	server, _ := newMockDtm(t, "/registerBranch")
	c := NewClient(WithServer(server))

	err := c.TccGlobalTransaction("tcc-gid-3", func(t *Tcc) error {
		return t.CallBranch("body", server+"/try", server+"/confirm", server+"/cancel")
	})
	if err == nil {
		t.Fatal("expected an error when registerBranch is rejected")
	}
}

// ---------------------------------------------------------------------------
// XA against the emulated server
// ---------------------------------------------------------------------------

func TestXaGlobalTransaction_Success(t *testing.T) {
	server, md := newMockDtm(t)
	c := NewClient(WithServer(server))

	var branchCalled bool
	err := c.XaGlobalTransaction("xa-gid-1", func(x *XA) error {
		err := x.CallBranch(map[string]string{"k": "v"}, server+"/api/xa-branch")
		branchCalled = err == nil
		return err
	})
	if err != nil {
		t.Fatalf("XaGlobalTransaction() error = %v", err)
	}
	if !branchCalled {
		t.Error("the XA branch was never called")
	}

	paths := md.seenPaths()
	if !paths["/prepare"] || !paths["/submit"] || !paths["/api/xa-branch"] {
		t.Errorf("expected /prepare, branch and /submit calls, got %v", md.seen())
	}
}

func TestXaGlobalTransaction_BranchErrorAborts(t *testing.T) {
	server, md := newMockDtm(t, "/api/xa-branch")
	c := NewClient(WithServer(server))

	err := c.XaGlobalTransaction("xa-gid-2", func(x *XA) error {
		return x.CallBranch("body", server+"/api/xa-branch")
	})
	if err == nil {
		t.Fatal("XaGlobalTransaction() expected an error when the branch fails")
	}

	paths := md.seenPaths()
	if !paths["/abort"] {
		t.Errorf("expected an /abort call, got %v", md.seen())
	}
	if paths["/submit"] {
		t.Error("a failed XA branch must not submit the transaction")
	}
}
