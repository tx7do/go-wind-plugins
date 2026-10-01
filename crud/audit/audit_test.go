package audit_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tx7do/go-wind-plugins/crud/audit"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

func TestOperationConstants(t *testing.T) {
	cases := []struct {
		name string
		op   audit.Operation
		want string
	}{
		{"OpInsert", audit.OpInsert, "INSERT"},
		{"OpUpdate", audit.OpUpdate, "UPDATE"},
		{"OpUpsert", audit.OpUpsert, "UPSERT"},
		{"OpDelete", audit.OpDelete, "DELETE"},
	}
	for _, tc := range cases {
		if string(tc.op) != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, string(tc.op), tc.want)
		}
	}
}

func TestStatusConstants(t *testing.T) {
	if audit.StatusOK != 0 {
		t.Errorf("StatusOK = %d, want 0", audit.StatusOK)
	}
	if audit.StatusFail != 1 {
		t.Errorf("StatusFail = %d, want 1", audit.StatusFail)
	}
}

// ---------------------------------------------------------------------------
// Entry value setters
// ---------------------------------------------------------------------------

type unmarshalable struct {
	C chan int // channels cannot be marshaled to JSON
}

func TestEntrySetPreValue(t *testing.T) {
	e := &audit.Entry{}
	if err := e.SetPreValue(map[string]any{"k": "v"}); err != nil {
		t.Fatalf("SetPreValue returned error: %v", err)
	}
	if len(e.PreValue) == 0 {
		t.Fatal("SetPreValue must populate PreValue")
	}
	var out map[string]any
	if err := json.Unmarshal(e.PreValue, &out); err != nil {
		t.Fatalf("PreValue is not valid JSON: %v", err)
	}
	if out["k"] != "v" {
		t.Errorf("PreValue[k] = %v, want %q", out["k"], "v")
	}
}

func TestEntrySetPreValueMarshalError(t *testing.T) {
	e := &audit.Entry{}
	if err := e.SetPreValue(unmarshalable{}); err == nil {
		t.Error("SetPreValue with unmarshalable value must return an error")
	}
}

func TestEntrySetPostValue(t *testing.T) {
	e := &audit.Entry{}
	if err := e.SetPostValue([]int{1, 2, 3}); err != nil {
		t.Fatalf("SetPostValue returned error: %v", err)
	}
	if len(e.PostValue) == 0 {
		t.Fatal("SetPostValue must populate PostValue")
	}
	var out []int
	if err := json.Unmarshal(e.PostValue, &out); err != nil {
		t.Fatalf("PostValue is not valid JSON: %v", err)
	}
	if len(out) != 3 || out[2] != 3 {
		t.Errorf("PostValue round-trip mismatch, got %v", out)
	}
}

func TestEntrySetPostValueMarshalError(t *testing.T) {
	e := &entryAlias{}
	if err := e.setPost(unmarshalable{}); err == nil {
		t.Error("SetPostValue with unmarshalable value must return an error")
	}
}

// entryAlias keeps the marshal-error test independent from SetPreValue testing.
type entryAlias struct {
	inner audit.Entry
}

func (a *entryAlias) setPost(v any) error {
	return a.inner.SetPostValue(v)
}

// ---------------------------------------------------------------------------
// Entry JSON serialization
// ---------------------------------------------------------------------------

func TestEntryJSONTags(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	e := &audit.Entry{
		TraceID:   "trace-1",
		Timestamp: ts,
		UserID:    42,
		TenantID:  7,
		Username:  "alice",
		Service:   "svc",
		Module:    "order",
		Action:    "Create",
		Resource:  "order_1",
		Operation: audit.OpUpdate,
		TargetID:  "123",
		Status:    audit.StatusFail,
		CostMS:    15,
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`"trace_id":"trace-1"`,
		`"user_id":42`,
		`"tenant_id":7`,
		`"username":"alice"`,
		`"service":"svc"`,
		`"operation":"UPDATE"`,
		`"target_id":"123"`,
		`"status":1`,
		`"cost_ms":15`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("serialized entry missing %s, got %s", want, s)
		}
	}
}

func TestEntryJSONOmitEmpty(t *testing.T) {
	// Zero values of omitempty fields must be omitted; Status has no
	// omitempty and must always be present.
	b, err := json.Marshal(&audit.Entry{Status: audit.StatusOK})
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	s := string(b)
	for _, unwanted := range []string{"user_id", "tenant_id", "username", "operation", "cost_ms"} {
		if strings.Contains(s, unwanted) {
			t.Errorf("zero-value field %q must be omitted, got %s", unwanted, s)
		}
	}
	if !strings.Contains(s, `"status":0`) {
		t.Errorf("Status must always be serialized, got %s", s)
	}
}

func TestEntryJSONRoundTrip(t *testing.T) {
	in := &audit.Entry{
		TraceID:  "t-1",
		UserID:   1,
		Extra:    map[string]any{"scope": "ALL"},
		PreValue: json.RawMessage(`{"a":1}`),
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out audit.Entry
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.TraceID != in.TraceID || out.UserID != in.UserID {
		t.Errorf("round-trip mismatch: got %+v, want trace=%q user=%d", out, in.TraceID, in.UserID)
	}
	if out.Extra["scope"] != "ALL" {
		t.Errorf("Extra round-trip mismatch: got %v", out.Extra)
	}
	if string(out.PreValue) != `{"a":1}` {
		t.Errorf("PreValue round-trip mismatch: got %s", string(out.PreValue))
	}
}

// ---------------------------------------------------------------------------
// Context propagation
// ---------------------------------------------------------------------------

// recordingAuditor is a minimal Auditor that captures entries for assertions.
type recordingAuditor struct {
	entries []*audit.Entry
	flushes int
}

func (r *recordingAuditor) Record(_ context.Context, e *audit.Entry) error {
	r.entries = append(r.entries, e)
	return nil
}

func (r *recordingAuditor) Flush(_ context.Context) error {
	r.flushes++
	return nil
}

func TestWithAuditorRoundTrip(t *testing.T) {
	a := &recordingAuditor{}
	ctx := audit.WithAuditor(context.Background(), a)

	got, ok := audit.FromContext(ctx)
	if !ok {
		t.Fatal("FromContext must find the auditor injected by WithAuditor")
	}
	rec, isRecorder := got.(*recordingAuditor)
	if !isRecorder {
		t.Fatalf("FromContext returned %T, want *recordingAuditor", got)
	}
	if err := rec.Record(ctx, &audit.Entry{Action: "test"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if len(rec.entries) != 1 || rec.entries[0].Action != "test" {
		t.Errorf("auditor did not record entry: %+v", rec.entries)
	}
}

func TestFromContextMissing(t *testing.T) {
	got, ok := audit.FromContext(context.Background())
	if ok {
		t.Errorf("FromContext on empty context must return ok=false, got %v", ok)
	}
	if got != nil {
		t.Errorf("FromContext on empty context must return nil auditor, got %T", got)
	}
}

func TestMustFromContextDefaults(t *testing.T) {
	// Missing auditor must fall back to a usable noop, not nil.
	a := audit.MustFromContext(context.Background())
	if a == nil {
		t.Fatal("MustFromContext must return a default auditor, got nil")
	}
	ctx := context.Background()
	if err := a.Record(ctx, &audit.Entry{}); err != nil {
		t.Errorf("noop Record must not fail, got %v", err)
	}
	if err := a.Flush(ctx); err != nil {
		t.Errorf("noop Flush must not fail, got %v", err)
	}
}

func TestMustFromContextNilContext(t *testing.T) {
	if a := audit.MustFromContext(nil); a == nil {
		t.Fatal("MustFromContext(nil) must return a default auditor, got nil")
	}
}

func TestMustFromContextReturnsInjected(t *testing.T) {
	a := &recordingAuditor{}
	ctx := audit.WithAuditor(context.Background(), a)
	got := audit.MustFromContext(ctx)
	if got != a {
		t.Errorf("MustFromContext must return the injected auditor, got %T", got)
	}
}
