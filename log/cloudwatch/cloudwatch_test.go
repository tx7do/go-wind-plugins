package cloudwatch

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"

	bLogger "github.com/tx7do/go-wind/log"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// staticCredsProvider returns fixed credentials so the SDK never performs
// credential discovery (no network, fully deterministic signing).
type staticCredsProvider struct{}

func (staticCredsProvider) Retrieve(context.Context) (aws.Credentials, error) {
	return aws.Credentials{AccessKeyID: "test-key", SecretAccessKey: "test-secret"}, nil
}

// putLogEventsBody mirrors the CloudWatch Logs PutLogEvents request body
// (AWS JSON 1.0 protocol).
type putLogEventsBody struct {
	LogGroupName  string `json:"logGroupName"`
	LogStreamName string `json:"logStreamName"`
	SequenceToken string `json:"sequenceToken"`
	LogEvents     []struct {
		Timestamp int64  `json:"timestamp"`
		Message   string `json:"message"`
	} `json:"logEvents"`
}

// cwServer is a local httptest server that captures PutLogEvents calls.
type cwServer struct {
	srv *httptest.Server

	mu       sync.Mutex
	requests []putLogEventsBody
	status   int // response status code, 200 by default
}

func newCWServer(t *testing.T) *cwServer {
	t.Helper()
	s := &cwServer{status: http.StatusOK}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading request body: %v", err)
		}

		var req putLogEventsBody
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("decoding PutLogEvents body %q: %v", body, err)
		}

		s.mu.Lock()
		s.requests = append(s.requests, req)
		status := s.status
		s.mu.Unlock()

		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write([]byte(`{"nextSequenceToken":"token-2"}`))
		} else {
			_, _ = w.Write([]byte(`{"__type":"BadRequestException","message":"rejected"}`))
		}
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func (s *cwServer) requestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

func (s *cwServer) lastRequest(t *testing.T) putLogEventsBody {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.requests) == 0 {
		t.Fatal("no requests received by test server")
	}
	return s.requests[len(s.requests)-1]
}

func (s *cwServer) setStatus(code int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = code
}

// newTestLogger builds a cloudwatchLog backed by a client pointed at the
// local test server. No AWS endpoint is contacted.
func newTestLogger(t *testing.T, s *cwServer, batchSize int) *cloudwatchLog {
	t.Helper()
	awsCfg := aws.Config{
		Region:           "us-east-1",
		Credentials:      staticCredsProvider{},
		BaseEndpoint:     aws.String(s.srv.URL),
		HTTPClient:       &http.Client{Timeout: 5 * time.Second},
		RetryMaxAttempts: 1,
		RetryMode:        aws.RetryModeStandard,
	}
	return &cloudwatchLog{
		client: cloudwatchlogs.NewFromConfig(awsCfg),
		opts:   &options{logGroup: "test-group", logStream: "test-stream", batchSize: batchSize, flushInterval: 0},
		st: &logState{
			flushCtx: context.Background(),
			cancel:   func() {},
		},
	}
}

func decodeEventMessage(t *testing.T, msg string) map[string]string {
	t.Helper()
	var m map[string]string
	if err := json.Unmarshal([]byte(msg), &m); err != nil {
		t.Fatalf("decoding event message %q: %v", msg, err)
	}
	return m
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

func TestDefaultOptions(t *testing.T) {
	cfg := defaultOptions()
	if cfg.region != "us-east-1" {
		t.Errorf("region = %q, want us-east-1", cfg.region)
	}
	if cfg.logGroup != "app" {
		t.Errorf("logGroup = %q, want app", cfg.logGroup)
	}
	if cfg.logStream != "default" {
		t.Errorf("logStream = %q, want default", cfg.logStream)
	}
	if cfg.batchSize != 100 {
		t.Errorf("batchSize = %d, want 100", cfg.batchSize)
	}
	if cfg.flushInterval != 5*time.Second {
		t.Errorf("flushInterval = %v, want 5s", cfg.flushInterval)
	}
}

func TestOptions_Appliers(t *testing.T) {
	cfg := defaultOptions()
	WithRegion("eu-central-1")(cfg)
	WithLogGroup("my-group")(cfg)
	WithLogStream("my-stream")(cfg)
	WithBatchSize(7)(cfg)
	WithFlushInterval(time.Second)(cfg)

	if cfg.region != "eu-central-1" {
		t.Errorf("region = %q", cfg.region)
	}
	if cfg.logGroup != "my-group" {
		t.Errorf("logGroup = %q", cfg.logGroup)
	}
	if cfg.logStream != "my-stream" {
		t.Errorf("logStream = %q", cfg.logStream)
	}
	if cfg.batchSize != 7 {
		t.Errorf("batchSize = %d, want 7", cfg.batchSize)
	}
	if cfg.flushInterval != time.Second {
		t.Errorf("flushInterval = %v, want 1s", cfg.flushInterval)
	}
}

// ---------------------------------------------------------------------------
// Batching, flush and payload shape
// ---------------------------------------------------------------------------

func TestLogger_Post_FlushesAtBatchSize(t *testing.T) {
	s := newCWServer(t)
	l := newTestLogger(t, s, 2)

	ctx := context.Background()

	// Below the batch threshold nothing is sent.
	l.Debug(ctx, "one", "k", "v")
	if got := s.requestCount(); got != 0 {
		t.Fatalf("expected 0 requests below batch threshold, got %d", got)
	}

	// The second entry crosses batchSize=2 and flushes synchronously.
	l.Error(ctx, "two")
	if got := s.requestCount(); got != 1 {
		t.Fatalf("expected 1 request after reaching batch size, got %d", got)
	}

	req := s.lastRequest(t)
	if req.LogGroupName != "test-group" {
		t.Errorf("logGroupName = %q, want %q", req.LogGroupName, "test-group")
	}
	if req.LogStreamName != "test-stream" {
		t.Errorf("logStreamName = %q, want %q", req.LogStreamName, "test-stream")
	}
	if len(req.LogEvents) != 2 {
		t.Fatalf("expected 2 log events, got %d", len(req.LogEvents))
	}
	for i, evt := range req.LogEvents {
		if evt.Timestamp <= 0 {
			t.Errorf("event[%d] timestamp = %d, want a positive millisecond value", i, evt.Timestamp)
		}
	}

	first := decodeEventMessage(t, req.LogEvents[0].Message)
	if first["level"] != "DEBUG" || first["msg"] != "one" || first["k"] != "v" {
		t.Errorf("first message = %v, want level DEBUG, msg one, k v", first)
	}
	second := decodeEventMessage(t, req.LogEvents[1].Message)
	if second["level"] != "ERROR" || second["msg"] != "two" {
		t.Errorf("second message = %v, want level ERROR, msg two", second)
	}

	// The response token is stored for the next write.
	if l.st.sequence == nil || *l.st.sequence != "token-2" {
		t.Errorf("sequence token = %v, want token-2", l.st.sequence)
	}
}

func TestLogger_Flush_UsesStoredSequenceToken(t *testing.T) {
	s := newCWServer(t)
	l := newTestLogger(t, s, 1)

	// First write: no token, response stores "token-2".
	l.Info(context.Background(), "first")
	if got := s.requestCount(); got != 1 {
		t.Fatalf("expected 1 request, got %d", got)
	}
	if got := s.lastRequest(t).SequenceToken; got != "" {
		t.Errorf("first request sequenceToken = %q, want empty", got)
	}

	// Second write must carry the stored token.
	l.Info(context.Background(), "second")
	if got := s.requestCount(); got != 2 {
		t.Fatalf("expected 2 requests, got %d", got)
	}
	if got := s.lastRequest(t).SequenceToken; got != "token-2" {
		t.Errorf("second request sequenceToken = %q, want %q", got, "token-2")
	}
}

func TestLogger_AllLevelsAndKeyvals(t *testing.T) {
	s := newCWServer(t)
	l := newTestLogger(t, s, 10) // no auto flush until Close

	ctx := context.Background()
	l.Debug(ctx, "d", "k", "v")
	l.Info(ctx, "i", "n", 42)
	l.Warn(ctx, "w")
	l.Error(ctx, "e")
	if got := s.requestCount(); got != 0 {
		t.Fatalf("expected 0 requests before flush, got %d", got)
	}

	l.flush()

	req := s.lastRequest(t)
	if len(req.LogEvents) != 4 {
		t.Fatalf("expected 4 log events, got %d", len(req.LogEvents))
	}

	want := []struct{ level, msg string }{
		{"DEBUG", "d"}, {"INFO", "i"}, {"WARN", "w"}, {"ERROR", "e"},
	}
	for i, w := range want {
		m := decodeEventMessage(t, req.LogEvents[i].Message)
		if m["level"] != w.level || m["msg"] != w.msg {
			t.Errorf("event[%d] = %v, want level %q msg %q", i, m, w.level, w.msg)
		}
	}
	if m := decodeEventMessage(t, req.LogEvents[0].Message); m["k"] != "v" {
		t.Errorf("event[0] k = %q, want v", m["k"])
	}
	if m := decodeEventMessage(t, req.LogEvents[1].Message); m["n"] != "42" {
		t.Errorf("event[1] n = %q, want 42", m["n"])
	}
}

// Odd trailing keyvals are dropped from the serialized message.
func TestLogger_OddKeyvalsDropped(t *testing.T) {
	s := newCWServer(t)
	l := newTestLogger(t, s, 10)

	l.Info(context.Background(), "hello", "lonely")
	l.flush()

	m := decodeEventMessage(t, s.lastRequest(t).LogEvents[0].Message)
	if _, ok := m["lonely"]; ok {
		t.Errorf("message %v should not contain the unpaired key", m)
	}
	if m["msg"] != "hello" {
		t.Errorf("message msg = %q, want %q", m["msg"], "hello")
	}
}

func TestLogger_Flush_EmptyBufferSendsNothing(t *testing.T) {
	s := newCWServer(t)
	l := newTestLogger(t, s, 10)

	l.flush() // buffer empty
	if got := s.requestCount(); got != 0 {
		t.Errorf("empty flush should not send requests, got %d", got)
	}
}

func TestLogger_Flush_ErrorStatusClearsBuffer(t *testing.T) {
	s := newCWServer(t)
	s.setStatus(http.StatusBadRequest)
	l := newTestLogger(t, s, 10)

	l.Info(context.Background(), "will fail")
	l.flush() // error is swallowed and logged, buffer must be cleared

	if got := s.requestCount(); got != 1 {
		t.Fatalf("expected 1 request attempt, got %d", got)
	}
	if len(l.st.buffer) != 0 {
		t.Errorf("buffer should be cleared after failed flush, has %d entries", len(l.st.buffer))
	}
	if l.st.sequence != nil {
		t.Errorf("sequence token should stay unset after failure, got %v", *l.st.sequence)
	}
}

func TestLogger_Close_FlushesRemainder(t *testing.T) {
	s := newCWServer(t)
	l := newTestLogger(t, s, 10)

	l.Info(context.Background(), "one")
	l.Info(context.Background(), "two")
	if got := s.requestCount(); got != 0 {
		t.Fatalf("expected no requests before Close, got %d", got)
	}

	if err := l.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
	if got := s.requestCount(); got != 1 {
		t.Fatalf("expected 1 request after Close, got %d", got)
	}
	if got := len(s.lastRequest(t).LogEvents); got != 2 {
		t.Errorf("expected 2 events after Close flush, got %d", got)
	}
}

func TestLogger_Close_EmptyBuffer(t *testing.T) {
	s := newCWServer(t)
	l := newTestLogger(t, s, 10)
	if err := l.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
	if got := s.requestCount(); got != 0 {
		t.Errorf("Close() with empty buffer should not send requests, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// With — persistent keyvals
// ---------------------------------------------------------------------------

// With() derived loggers share the flush state with their parent, so events
// written through the child are flushed by the shared sequence-token state.
func TestLogger_With_CarriesExtras(t *testing.T) {
	s := newCWServer(t)
	l := newTestLogger(t, s, 10)
	child, ok := l.With("module", "api").(*cloudwatchLog)
	if !ok {
		t.Fatalf("With() returned %T, want *cloudwatchLog", l.With("module", "api"))
	}

	if child.client == nil {
		t.Error("child logger should inherit the parent's client")
	}
	if child.opts != l.opts {
		t.Error("child logger should share the parent's options")
	}
	if child.st != l.st {
		t.Error("child logger should share the parent's flush state")
	}

	child.Error(context.Background(), "boom", "attempt", 3)
	// The root logger stays untouched by the child's keyvals.
	l.Error(context.Background(), "root")

	if err := l.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := s.requestCount(); got != 1 {
		t.Fatalf("expected 1 flush request, got %d", got)
	}

	req := s.lastRequest(t)
	if len(req.LogEvents) != 2 {
		t.Fatalf("expected 2 log events, got %d", len(req.LogEvents))
	}

	childMsg := decodeEventMessage(t, req.LogEvents[0].Message)
	if childMsg["module"] != "api" || childMsg["attempt"] != "3" {
		t.Errorf("child message = %v, want module=api and attempt=3", childMsg)
	}
	if childMsg["msg"] != "boom" {
		t.Errorf("child message msg = %q, want %q", childMsg["msg"], "boom")
	}

	rootMsg := decodeEventMessage(t, req.LogEvents[1].Message)
	if _, ok := rootMsg["module"]; ok {
		t.Errorf("root logger message %v should not contain the child keyvals", rootMsg)
	}
}

// ---------------------------------------------------------------------------
// Enabled
// ---------------------------------------------------------------------------

func TestLogger_Enabled_AlwaysTrue(t *testing.T) {
	s := newCWServer(t)
	l := newTestLogger(t, s, 10)
	for _, level := range []bLogger.Level{bLogger.LevelDebug, bLogger.LevelInfo, bLogger.LevelWarn, bLogger.LevelError} {
		if !l.Enabled(level) {
			t.Errorf("Enabled(%v) = false, want true", level)
		}
	}
}

// ---------------------------------------------------------------------------
// Error classification helpers
// ---------------------------------------------------------------------------

func TestIsSequenceTokenErr(t *testing.T) {
	typed := &types.InvalidSequenceTokenException{Message: aws.String("bad token")}
	if !isSequenceTokenErr(typed) {
		t.Error("typed InvalidSequenceTokenException should be recognized")
	}
	if !isSequenceTokenErr(errors.New("…InvalidSequenceTokenException somewhere")) {
		t.Error("string match should recognize the exception name")
	}
	if isSequenceTokenErr(errors.New("some other failure")) {
		t.Error("unrelated error should not be classified as a sequence token error")
	}
}

func TestIsAlreadyExists(t *testing.T) {
	typed := &types.ResourceAlreadyExistsException{Message: aws.String("already there")}
	if !isAlreadyExists(typed) {
		t.Error("typed ResourceAlreadyExistsException should be recognized")
	}
	if !isAlreadyExists(errors.New("…ResourceAlreadyExistsException somewhere")) {
		t.Error("string match should recognize the exception name")
	}
	if isAlreadyExists(errors.New("some other failure")) {
		t.Error("unrelated error should not be classified as already-exists")
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
		{[]int{1, 2}, "[1,2]"}, // fall back to JSON encoding
		{map[string]int{"a": 1}, `{"a":1}`},
	}

	for _, test := range tests {
		if got := toString(test.in); got != test.want {
			t.Errorf("toString(%#v) = %q, want %q", test.in, got, test.want)
		}
	}
}
