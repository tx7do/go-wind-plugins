package dtm

import (
	"net/url"
	"testing"

	"github.com/tx7do/go-wind-plugins/transaction"
)

// ---------------------------------------------------------------------------
// Client construction and options
// ---------------------------------------------------------------------------

func TestNewClient_DefaultServer(t *testing.T) {
	c := NewClient()

	if c.Server() != defaultServer {
		t.Errorf("Server() = %q, want default %q", c.Server(), defaultServer)
	}
	if c.Server() != "http://localhost:36789/api/dtmsvr" {
		t.Errorf("Server() = %q, want the documented default address", c.Server())
	}
}

func TestNewClient_WithServer(t *testing.T) {
	c := NewClient(WithServer("http://dtm.example.com:36790/api/dtmsvr"))

	if c.Server() != "http://dtm.example.com:36790/api/dtmsvr" {
		t.Errorf("Server() = %q, want the WithServer value", c.Server())
	}
}

func TestWithServer_LastOptionWins(t *testing.T) {
	c := NewClient(WithServer("http://a"), WithServer("http://b"))

	if c.Server() != "http://b" {
		t.Errorf("Server() = %q, want http://b (last option wins)", c.Server())
	}
}

func TestClientClose_ReturnsNil(t *testing.T) {
	c := NewClient()
	if err := c.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

// Compile-time: the DTM client must satisfy the shared transaction.Client.
var _ transaction.Client = (*Client)(nil)

func TestClient_ImplementsTransactionClient(t *testing.T) {
	var c transaction.Client = NewClient()
	if err := c.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// Saga builder (config assembly only — no server interaction)
// ---------------------------------------------------------------------------

func TestNewSaga_ChainedSteps(t *testing.T) {
	c := NewClient()

	s := c.NewSaga("saga-gid")
	if s == nil {
		t.Fatal("NewSaga() returned nil")
	}

	payload := map[string]string{"amount": "100"}
	ret := s.Add("/api/transfer/out", "/api/transfer/out/compensate", payload).
		Add("/api/transfer/in", "/api/transfer/in/compensate", payload)

	// Chaining must return the same builder.
	if ret != s {
		t.Error("Add() must return the same *Saga for chaining")
	}

	// Steps are recorded in the underlying dtmcli transaction.
	if len(s.saga.Steps) != 2 {
		t.Fatalf("saga steps = %d, want 2", len(s.saga.Steps))
	}
	if s.saga.Steps[0]["action"] != "/api/transfer/out" {
		t.Errorf("step 0 action = %q, want /api/transfer/out", s.saga.Steps[0]["action"])
	}
	if s.saga.Steps[0]["compensate"] != "/api/transfer/out/compensate" {
		t.Errorf("step 0 compensate = %q, want /api/transfer/out/compensate", s.saga.Steps[0]["compensate"])
	}
	if s.saga.Steps[1]["action"] != "/api/transfer/in" {
		t.Errorf("step 1 action = %q, want /api/transfer/in", s.saga.Steps[1]["action"])
	}
	if len(s.saga.Payloads) != 2 {
		t.Errorf("saga payloads = %d, want 2", len(s.saga.Payloads))
	}
	// The payload must be JSON-encoded by the builder.
	if s.saga.Payloads[0] != `{"amount":"100"}` {
		t.Errorf("payload[0] = %v, want JSON encoding of the data", s.saga.Payloads[0])
	}
}

func TestNewSaga_GidAndTransType(t *testing.T) {
	c := NewClient(WithServer("http://dtm:36789/api/dtmsvr"))

	s := c.NewSaga("my-gid")
	if s.saga.Gid != "my-gid" {
		t.Errorf("Gid = %q, want my-gid", s.saga.Gid)
	}
	if s.saga.TransType != "saga" {
		t.Errorf("TransType = %q, want saga", s.saga.TransType)
	}
	if s.saga.Dtm != "http://dtm:36789/api/dtmsvr" {
		t.Errorf("Dtm = %q, want the client's configured server", s.saga.Dtm)
	}
}

func TestSaga_SetConcurrent(t *testing.T) {
	c := NewClient()
	s := c.NewSaga("gid")

	ret := s.SetConcurrent()
	if ret != s {
		t.Error("SetConcurrent must return the same *Saga for chaining")
	}
	if !s.saga.Concurrent {
		t.Error("SetConcurrent() did not enable concurrent execution")
	}
}

// ---------------------------------------------------------------------------
// Msg builder (config assembly only — no server interaction)
// ---------------------------------------------------------------------------

func TestNewMsg_ChainedSteps(t *testing.T) {
	c := NewClient()

	m := c.NewMsg("msg-gid")
	if m == nil {
		t.Fatal("NewMsg() returned nil")
	}

	ret := m.Add("/api/send-email", "body1").
		AddTopic("orders-topic", "body2").
		SetDelay(30)

	if ret != m {
		t.Error("Add/AddTopic/SetDelay must return the same *Msg for chaining")
	}

	// Add and AddTopic each append one step (AddTopic prefixes the URL).
	if len(m.msg.Steps) != 2 {
		t.Fatalf("msg steps = %d, want 2", len(m.msg.Steps))
	}
	if m.msg.Steps[0]["action"] != "/api/send-email" {
		t.Errorf("step 0 action = %q, want /api/send-email", m.msg.Steps[0]["action"])
	}
	if m.msg.Steps[1]["action"] != "topic://orders-topic" {
		t.Errorf("step 1 action = %q, want topic://orders-topic", m.msg.Steps[1]["action"])
	}
	if len(m.msg.Payloads) != 2 {
		t.Errorf("msg payloads = %d, want 2", len(m.msg.Payloads))
	}
}

func TestNewMsg_GidAndTransType(t *testing.T) {
	c := NewClient(WithServer("http://dtm:36789/api/dtmsvr"))

	m := c.NewMsg("my-msg")
	if m.msg.Gid != "my-msg" {
		t.Errorf("Gid = %q, want my-msg", m.msg.Gid)
	}
	if m.msg.TransType != "msg" {
		t.Errorf("TransType = %q, want msg", m.msg.TransType)
	}
}

// ---------------------------------------------------------------------------
// BarrierFromQuery
// ---------------------------------------------------------------------------

func TestBarrierFromQuery_Valid(t *testing.T) {
	qs := url.Values{
		"trans_type": {"saga"},
		"gid":        {"gid-1"},
		"branch_id":  {"br-1"},
		"op":         {"action"},
	}

	bb, err := BarrierFromQuery(qs)
	if err != nil {
		t.Fatalf("BarrierFromQuery() error = %v", err)
	}
	if bb.TransType != "saga" {
		t.Errorf("TransType = %q, want saga", bb.TransType)
	}
	if bb.Gid != "gid-1" {
		t.Errorf("Gid = %q, want gid-1", bb.Gid)
	}
	if bb.BranchID != "br-1" {
		t.Errorf("BranchID = %q, want br-1", bb.BranchID)
	}
	if bb.Op != "action" {
		t.Errorf("Op = %q, want action", bb.Op)
	}
}

func TestBarrierFromQuery_MissingFieldsError(t *testing.T) {
	tests := []struct {
		name string
		qs   url.Values
	}{
		{"empty", url.Values{}},
		{"missing op", url.Values{"trans_type": {"saga"}, "gid": {"g"}, "branch_id": {"b"}}},
		{"missing gid", url.Values{"trans_type": {"saga"}, "branch_id": {"b"}, "op": {"action"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := BarrierFromQuery(tt.qs); err == nil {
				t.Error("BarrierFromQuery() with incomplete query should fail")
			}
		})
	}
}
