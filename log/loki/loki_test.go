package loki

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	bLogger "github.com/tx7do/go-wind/log"
)

// lokiPayload mirrors the Loki push API request body.
type lokiPayload struct {
	Streams []struct {
		Stream map[string]string `json:"stream"`
		Values [][]string        `json:"values"`
	} `json:"streams"`
}

// capturedRequest records one HTTP request received by the test server.
type capturedRequest struct {
	body []byte
}

// testServer is a local HTTP server capturing Loki push payloads.
type testServer struct {
	srv *httptest.Server

	mu       sync.Mutex
	requests []capturedRequest
	status   int // response status code, 200 by default
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	ts := &testServer{status: http.StatusOK}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		buf, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading request body: %v", err)
		}

		ts.mu.Lock()
		ts.requests = append(ts.requests, capturedRequest{body: buf})
		status := ts.status
		ts.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte("{}"))
	})
	ts.srv = httptest.NewServer(mux)
	t.Cleanup(ts.srv.Close)
	return ts
}

func (ts *testServer) requestCount() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return len(ts.requests)
}

func (ts *testServer) lastPayload(t *testing.T) lokiPayload {
	t.Helper()
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if len(ts.requests) == 0 {
		t.Fatal("no requests received by test server")
	}
	var p lokiPayload
	if err := json.Unmarshal(ts.requests[len(ts.requests)-1].body, &p); err != nil {
		t.Fatalf("decoding payload: %v (body: %s)", err, ts.requests[len(ts.requests)-1].body)
	}
	return p
}

func (ts *testServer) setStatus(code int) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.status = code
}

// newTestLogger builds a lokiLog wired to the local test server.
func newTestLogger(t *testing.T, ts *testServer, extra ...Option) Logger {
	t.Helper()
	opts := append([]Option{
		WithEndpoint(ts.srv.URL),
		WithBatchSize(1),
	}, extra...)
	l, err := NewLogger(opts...)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	return l
}

func decodeLine(t *testing.T, line string) map[string]string {
	t.Helper()
	var m map[string]string
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("decoding log line %q: %v", line, err)
	}
	return m
}

// ---------------------------------------------------------------------------
// Constructor and options
// ---------------------------------------------------------------------------

func TestNewLogger_RequiresEndpoint(t *testing.T) {
	if _, err := NewLogger(); err == nil {
		t.Error("NewLogger() without endpoint should return an error")
	}
}

func TestNewLogger_Defaults(t *testing.T) {
	cfg := defaultOptions()
	if cfg.endpoint != "" {
		t.Errorf("default endpoint = %q, want empty", cfg.endpoint)
	}
	if cfg.batchSize != 100 {
		t.Errorf("default batchSize = %d, want 100", cfg.batchSize)
	}
	if cfg.flushInterval != 5*time.Second {
		t.Errorf("default flushInterval = %v, want 5s", cfg.flushInterval)
	}
	if got := cfg.labels["app"]; got != "default" {
		t.Errorf("default labels[app] = %q, want %q", got, "default")
	}
	if cfg.httpClient != nil {
		t.Errorf("default httpClient = %v, want nil", cfg.httpClient)
	}
}

func TestOptions_Appliers(t *testing.T) {
	client := &http.Client{Timeout: time.Second}
	cfg := defaultOptions()
	WithEndpoint("http://example.com/push")(cfg)
	WithLabel("app", "my-service")(cfg)
	WithLabel("env", "production")(cfg)
	WithBatchSize(7)(cfg)
	WithFlushInterval(3 * time.Second)(cfg)
	WithHTTPClient(client)(cfg)

	if cfg.endpoint != "http://example.com/push" {
		t.Errorf("endpoint = %q", cfg.endpoint)
	}
	if cfg.labels["app"] != "my-service" || cfg.labels["env"] != "production" {
		t.Errorf("labels = %v", cfg.labels)
	}
	if cfg.batchSize != 7 {
		t.Errorf("batchSize = %d, want 7", cfg.batchSize)
	}
	if cfg.flushInterval != 3*time.Second {
		t.Errorf("flushInterval = %v, want 3s", cfg.flushInterval)
	}
	if cfg.httpClient != client {
		t.Errorf("httpClient not applied")
	}
}

// ---------------------------------------------------------------------------
// Log emission, batching and payload shape
// ---------------------------------------------------------------------------

func TestLogger_EmitsAndBatches(t *testing.T) {
	ts := newTestServer(t)
	l := newTestLogger(t, ts, WithBatchSize(3), WithLabel("app", "test"))

	ctx := context.Background()

	// Below the batch threshold nothing is sent.
	l.Debug(ctx, "one", "k", "v")
	l.Info(ctx, "two")
	if got := ts.requestCount(); got != 0 {
		t.Fatalf("expected 0 requests below batch threshold, got %d", got)
	}

	// The third entry crosses batchSize=3 and flushes synchronously.
	l.Error(ctx, "three")
	if got := ts.requestCount(); got != 1 {
		t.Fatalf("expected 1 request after reaching batch size, got %d", got)
	}

	payload := ts.lastPayload(t)
	if len(payload.Streams) != 1 {
		t.Fatalf("expected 1 stream, got %d", len(payload.Streams))
	}
	stream := payload.Streams[0]
	if stream.Stream["app"] != "test" {
		t.Errorf("stream label app = %q, want %q", stream.Stream["app"], "test")
	}
	if len(stream.Values) != 3 {
		t.Fatalf("expected 3 values in the batch, got %d", len(stream.Values))
	}

	// Each value is [nanosecond-timestamp, JSON line].
	for i, v := range stream.Values {
		if len(v) != 2 {
			t.Fatalf("value[%d] has %d parts, want 2", i, len(v))
		}
		if _, err := strconv.ParseInt(v[0], 10, 64); err != nil {
			t.Errorf("value[%d] timestamp %q is not a nanosecond integer: %v", i, v[0], err)
		}
		decodeLine(t, v[1]) // must be a valid JSON object
	}

	first := decodeLine(t, stream.Values[0][1])
	if first["level"] != "DEBUG" || first["msg"] != "one" || first["k"] != "v" {
		t.Errorf("first line = %v, want level DEBUG, msg one, k v", first)
	}
	second := decodeLine(t, stream.Values[1][1])
	if second["level"] != "INFO" || second["msg"] != "two" {
		t.Errorf("second line = %v, want level INFO, msg two", second)
	}
	third := decodeLine(t, stream.Values[2][1])
	if third["level"] != "ERROR" || third["msg"] != "three" {
		t.Errorf("third line = %v, want level ERROR, msg three", third)
	}
}

func TestLogger_EachLevelEmitted(t *testing.T) {
	ts := newTestServer(t)
	l := newTestLogger(t, ts, WithBatchSize(4))

	ctx := context.Background()
	l.Debug(ctx, "d")
	l.Info(ctx, "i")
	l.Warn(ctx, "w")
	l.Error(ctx, "e")

	if got := ts.requestCount(); got != 1 {
		t.Fatalf("expected 1 batched request, got %d", got)
	}
	values := ts.lastPayload(t).Streams[0].Values
	want := []struct{ level, msg string }{
		{"DEBUG", "d"}, {"INFO", "i"}, {"WARN", "w"}, {"ERROR", "e"},
	}
	for i, w := range want {
		line := decodeLine(t, values[i][1])
		if line["level"] != w.level || line["msg"] != w.msg {
			t.Errorf("value[%d] = level %q msg %q, want %q/%q",
				i, line["level"], line["msg"], w.level, w.msg)
		}
	}
}

// Odd trailing keyvals are dropped from the serialized line.
func TestLogger_OddKeyvalsDropped(t *testing.T) {
	ts := newTestServer(t)
	l := newTestLogger(t, ts)

	l.Info(context.Background(), "hello", "lonely")

	payload := ts.lastPayload(t)
	line := decodeLine(t, payload.Streams[0].Values[0][1])
	if _, ok := line["lonely"]; ok {
		t.Errorf("line %v should not contain the unpaired key", line)
	}
	if line["msg"] != "hello" {
		t.Errorf("line msg = %q, want %q", line["msg"], "hello")
	}
}

// Close flushes whatever is left in the buffer.
func TestLogger_Close_FlushesRemainder(t *testing.T) {
	ts := newTestServer(t)
	l := newTestLogger(t, ts, WithBatchSize(10))

	ctx := context.Background()
	l.Info(ctx, "one")
	l.Info(ctx, "two")
	if got := ts.requestCount(); got != 0 {
		t.Fatalf("expected no flush before Close, got %d requests", got)
	}

	if err := l.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := ts.requestCount(); got != 1 {
		t.Fatalf("expected 1 request after Close, got %d", got)
	}
	if got := len(ts.lastPayload(t).Streams[0].Values); got != 2 {
		t.Errorf("expected 2 values after Close flush, got %d", got)
	}

	// A second Close has nothing left to send.
	if err := l.Close(); err != nil {
		t.Errorf("second Close() error = %v", err)
	}
	if got := ts.requestCount(); got != 1 {
		t.Errorf("second Close() should not send anything, got %d requests", got)
	}
}

// ---------------------------------------------------------------------------
// With — persistent keyvals
// ---------------------------------------------------------------------------

func TestLogger_With_CarriesExtras(t *testing.T) {
	ts := newTestServer(t)
	l := newTestLogger(t, ts)
	child := l.With("module", "registry")

	child.Error(context.Background(), "boom", "attempt", 3)

	payload := ts.lastPayload(t)
	line := decodeLine(t, payload.Streams[0].Values[0][1])
	if line["module"] != "registry" || line["attempt"] != "3" {
		t.Errorf("line = %v, want module=registry and attempt=3", line)
	}
	if line["msg"] != "boom" {
		t.Errorf("line msg = %q, want %q", line["msg"], "boom")
	}
}

func TestLogger_With_Chained(t *testing.T) {
	ts := newTestServer(t)
	l := newTestLogger(t, ts)
	child := l.With("a", "1").With("b", "2")

	child.Warn(context.Background(), "m")

	line := decodeLine(t, ts.lastPayload(t).Streams[0].Values[0][1])
	if line["a"] != "1" || line["b"] != "2" {
		t.Errorf("line = %v, want a=1 and b=2", line)
	}
}

// ---------------------------------------------------------------------------
// Error handling
// ---------------------------------------------------------------------------

func TestLogger_Close_ReturnsErrorOnBadStatus(t *testing.T) {
	ts := newTestServer(t)
	ts.setStatus(http.StatusBadRequest)
	l := newTestLogger(t, ts, WithBatchSize(10))

	l.Info(context.Background(), "will fail to ship")
	if err := l.Close(); err == nil {
		t.Error("Close() should propagate the flush error when the server returns 400")
	}
}

// ---------------------------------------------------------------------------
// Enabled
// ---------------------------------------------------------------------------

func TestLogger_Enabled_AlwaysTrue(t *testing.T) {
	ts := newTestServer(t)
	l := newTestLogger(t, ts)
	for _, level := range []bLogger.Level{bLogger.LevelDebug, bLogger.LevelInfo, bLogger.LevelWarn, bLogger.LevelError} {
		if !l.Enabled(level) {
			t.Errorf("Enabled(%v) = false, want true", level)
		}
	}
}

// ---------------------------------------------------------------------------
// toString — the pure value formatter
// ---------------------------------------------------------------------------

type stringerValue struct{}

func (stringerValue) String() string { return "stringer!" }

func TestToString(t *testing.T) {
	tests := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"text", "text"},
		{42, "42"},
		{int8(-8), "-8"},
		{int16(-16), "-16"},
		{int32(-32), "-32"},
		{int64(-64), "-64"},
		{uint(5), "5"},
		{uint8(8), "8"},
		{uint16(16), "16"},
		{uint32(32), "32"},
		{uint64(64), "64"},
		{3.5, "3.5"},
		{float32(1.5), "1.5"},
		{true, "true"},
		{false, "false"},
		{[]byte("bytes"), "bytes"},
		{stringerValue{}, "stringer!"},
		{errors.New("boom"), "{}"}, // error does not implement fmt.Stringer; JSON fallback
		{[]int{1, 2}, "[1,2]"},     // fall back to JSON encoding
		{map[string]int{"a": 1}, `{"a":1}`},
	}

	for _, test := range tests {
		if got := toString(test.in); got != test.want {
			t.Errorf("toString(%#v) = %q, want %q", test.in, got, test.want)
		}
	}
}

// An empty message still produces a well-formed line with a msg field.
func TestLogger_EmptyMessage(t *testing.T) {
	ts := newTestServer(t)
	l := newTestLogger(t, ts)

	l.Info(context.Background(), "")

	payload := ts.lastPayload(t)
	line := decodeLine(t, payload.Streams[0].Values[0][1])
	if _, ok := line["msg"]; !ok {
		t.Errorf("line %v should always contain the msg field", line)
	}
	if line["level"] != "INFO" {
		t.Errorf("line level = %q, want INFO", line["level"])
	}
}
