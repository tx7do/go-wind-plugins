package sentry

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"

	bLogger "github.com/tx7do/go-wind/log"
)

// ---------------------------------------------------------------------------
// recordingTransport — a deterministic in-memory Sentry transport
// ---------------------------------------------------------------------------

type recordingTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (t *recordingTransport) SendEvent(e *sentry.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, e)
}

func (t *recordingTransport) Flush(time.Duration) bool              { return true }
func (t *recordingTransport) FlushWithContext(context.Context) bool { return true }
func (t *recordingTransport) Close()                                {}
func (t *recordingTransport) Configure(sentry.ClientOptions)        {}

var _ sentry.Transport = (*recordingTransport)(nil)

func (t *recordingTransport) collected() []*sentry.Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]*sentry.Event{}, t.events...)
}

// newTestLogger builds a sentryLog with a client that records events through
// the given transport. No network is involved.
func newTestLogger(t *testing.T) (*sentryLog, *recordingTransport) {
	t.Helper()
	tr := &recordingTransport{}
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:       "https://public@example.com/1",
		Transport: tr,
	})
	if err != nil {
		t.Fatalf("sentry.NewClient() error = %v", err)
	}
	hub := sentry.NewHub(client, sentry.NewScope())
	return &sentryLog{hub: hub}, tr
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

func TestNewLogger_RequiresDSN(t *testing.T) {
	if _, err := NewLogger(); err == nil {
		t.Error("NewLogger() without DSN should return an error")
	}
}

func TestNewLogger_InvalidDSN(t *testing.T) {
	if _, err := NewLogger(WithDSN("not-a-valid-dsn")); err == nil {
		t.Error("NewLogger() with an invalid DSN should return an error")
	}
}

func TestNewLogger_ValidDSN(t *testing.T) {
	// A syntactically valid DSN initializes the SDK without any network
	// activity; delivery only happens when an event is captured.
	l, err := NewLogger(WithDSN("https://publickey@o123.ingest.example.com/456"))
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	if l == nil {
		t.Fatal("NewLogger() returned nil")
	}
	if l.GetHub() == nil {
		t.Error("GetHub() should return the initialized hub")
	}
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

func TestOptions_Appliers(t *testing.T) {
	cfg := defaultOptions()
	if cfg.environment != "development" {
		t.Errorf("default environment = %q, want %q", cfg.environment, "development")
	}

	WithDSN("https://k@example.com/1")(cfg)
	WithEnvironment("production")(cfg)
	WithRelease("myapp@1.0.0")(cfg)
	WithServerName("host-1")(cfg)

	if cfg.dsn != "https://k@example.com/1" {
		t.Errorf("dsn = %q", cfg.dsn)
	}
	if cfg.environment != "production" {
		t.Errorf("environment = %q", cfg.environment)
	}
	if cfg.release != "myapp@1.0.0" {
		t.Errorf("release = %q", cfg.release)
	}
	if cfg.serverName != "host-1" {
		t.Errorf("serverName = %q", cfg.serverName)
	}
}

// ---------------------------------------------------------------------------
// Error path — events are captured through the transport synchronously
// ---------------------------------------------------------------------------

func TestLogger_Error_CapturesEvent(t *testing.T) {
	l, tr := newTestLogger(t)

	l.Error(nil, "boom", "k", "v")

	events := tr.collected()
	if len(events) != 1 {
		t.Fatalf("expected 1 captured event, got %d", len(events))
	}
	evt := events[0]
	if evt.Level != sentry.LevelError {
		t.Errorf("event level = %v, want error", evt.Level)
	}
	if evt.Message != "boom" {
		t.Errorf("event message = %q, want %q", evt.Message, "boom")
	}
	if got, ok := evt.Contexts["k"]["value"]; !ok || got != "v" {
		t.Errorf("event contexts[k][value] = %v, want %q", got, "v")
	}
}

// Breadcrumbs recorded by Debug/Info/Warn are attached to the next event.
func TestLogger_BreadcrumbsAttachedToEvent(t *testing.T) {
	l, tr := newTestLogger(t)

	l.Debug(nil, "step one", "i", 1)
	l.Info(nil, "step two")
	l.Warn(nil, "careful")
	l.Error(nil, "boom")

	events := tr.collected()
	if len(events) != 1 {
		t.Fatalf("expected 1 captured event (only Error), got %d", len(events))
	}

	bcs := events[0].Breadcrumbs
	if len(bcs) != 3 {
		t.Fatalf("expected 3 breadcrumbs on the event, got %d", len(bcs))
	}

	wantLevels := []sentry.Level{sentry.LevelDebug, sentry.LevelInfo, sentry.LevelWarning}
	wantMsgs := []string{"step one", "step two", "careful"}
	for i, bc := range bcs {
		if bc.Level != wantLevels[i] {
			t.Errorf("breadcrumb[%d] level = %v, want %v", i, bc.Level, wantLevels[i])
		}
		if bc.Message != wantMsgs[i] {
			t.Errorf("breadcrumb[%d] message = %q, want %q", i, bc.Message, wantMsgs[i])
		}
		if bc.Type != "default" {
			t.Errorf("breadcrumb[%d] type = %q, want %q", i, bc.Type, "default")
		}
	}
	if got, ok := bcs[0].Data["i"]; !ok || got != 1 {
		t.Errorf("breadcrumb[0] data[i] = %v, want 1", got)
	}
}

// ---------------------------------------------------------------------------
// breadcrumb / event builders — pure functions
// ---------------------------------------------------------------------------

func TestBreadcrumb_Build(t *testing.T) {
	s := &sentryLog{}
	bc := s.breadcrumb(sentry.LevelWarning, "watch out", []any{"k", "v", "n", 2})

	if bc.Type != "default" {
		t.Errorf("Type = %q, want %q", bc.Type, "default")
	}
	if bc.Level != sentry.LevelWarning {
		t.Errorf("Level = %v, want warning", bc.Level)
	}
	if bc.Message != "watch out" {
		t.Errorf("Message = %q, want %q", bc.Message, "watch out")
	}
	if bc.Data == nil {
		t.Fatal("Data map should be initialized")
	}
	if got := bc.Data["k"]; got != "v" {
		t.Errorf(`Data["k"] = %v, want "v"`, got)
	}
	if got := bc.Data["n"]; got != 2 {
		t.Errorf(`Data["n"] = %v, want 2`, got)
	}
}

func TestBreadcrumb_OddKeyvalsDropped(t *testing.T) {
	s := &sentryLog{}
	bc := s.breadcrumb(sentry.LevelInfo, "m", []any{"k", "v", "lonely"})

	if got, ok := bc.Data["lonely"]; ok {
		t.Errorf(`Data["lonely"] = %v, want no entry`, got)
	}
	if got := bc.Data["k"]; got != "v" {
		t.Errorf(`Data["k"] = %v, want "v"`, got)
	}
}

func TestEvent_Build(t *testing.T) {
	s := &sentryLog{}
	evt := s.event(sentry.LevelError, "boom", []any{"k", "v", "n", 42})

	if evt.Level != sentry.LevelError {
		t.Errorf("Level = %v, want error", evt.Level)
	}
	if evt.Message != "boom" {
		t.Errorf("Message = %q, want %q", evt.Message, "boom")
	}
	if got, ok := evt.Contexts["k"]["value"]; !ok || got != "v" {
		t.Errorf(`Contexts["k"]["value"] = %v, want "v"`, got)
	}
	if got, ok := evt.Contexts["n"]["value"]; !ok || got != 42 {
		t.Errorf(`Contexts["n"]["value"] = %v, want 42`, got)
	}
}

func TestEvent_OddKeyvalsDropped(t *testing.T) {
	s := &sentryLog{}
	evt := s.event(sentry.LevelError, "boom", []any{"k", "v", "lonely"})

	if _, ok := evt.Contexts["lonely"]; ok {
		t.Error("unpaired key should not end up in Contexts")
	}
}

func TestEvent_DuplicateKeyKeepsLastValue(t *testing.T) {
	s := &sentryLog{}
	evt := s.event(sentry.LevelError, "boom", []any{"k", "first", "k", "second"})

	// The adapter merges into the existing context entry.
	if got, ok := evt.Contexts["k"]["value"]; !ok || got != "second" {
		t.Errorf(`Contexts["k"]["value"] = %v, want "second"`, got)
	}
}

// ---------------------------------------------------------------------------
// With — persistent keyvals
// ---------------------------------------------------------------------------

func TestLogger_With_ExtrasInEvent(t *testing.T) {
	l, tr := newTestLogger(t)
	child := l.With("module", "api")

	child.Error(nil, "boom")

	events := tr.collected()
	if len(events) != 1 {
		t.Fatalf("expected 1 captured event, got %d", len(events))
	}
	if got, ok := events[0].Contexts["module"]["value"]; !ok || got != "api" {
		t.Errorf(`Contexts["module"]["value"] = %v, want "api"`, got)
	}
}

func TestLogger_With_Chained(t *testing.T) {
	l, tr := newTestLogger(t)
	child := l.With("a", "1").With("b", "2")

	child.Error(nil, "m")

	events := tr.collected()
	if len(events) != 1 {
		t.Fatalf("expected 1 captured event, got %d", len(events))
	}
	ctx := events[0].Contexts
	if got, ok := ctx["a"]["value"]; !ok || got != "1" {
		t.Errorf(`Contexts["a"]["value"] = %v, want "1"`, got)
	}
	if got, ok := ctx["b"]["value"]; !ok || got != "2" {
		t.Errorf(`Contexts["b"]["value"] = %v, want "2"`, got)
	}
}

// The root logger stays untouched by With.
func TestLogger_With_RootUnchanged(t *testing.T) {
	l, tr := newTestLogger(t)
	_ = l.With("module", "api")

	l.Error(nil, "root")

	events := tr.collected()
	if len(events) != 1 {
		t.Fatalf("expected 1 captured event, got %d", len(events))
	}
	if _, ok := events[0].Contexts["module"]; ok {
		t.Error("root logger event should not contain the child keyvals")
	}
}

// ---------------------------------------------------------------------------
// Enabled, Close, GetHub, levelToInt
// ---------------------------------------------------------------------------

func TestLogger_Enabled_AlwaysTrue(t *testing.T) {
	l, _ := newTestLogger(t)
	for _, level := range []bLogger.Level{bLogger.LevelDebug, bLogger.LevelInfo, bLogger.LevelWarn, bLogger.LevelError} {
		if !l.Enabled(level) {
			t.Errorf("Enabled(%v) = false, want true", level)
		}
	}
}

func TestLogger_Close(t *testing.T) {
	l, _ := newTestLogger(t)
	if err := l.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

func TestLogger_GetHub(t *testing.T) {
	l, _ := newTestLogger(t)
	hub := l.GetHub()
	if hub == nil {
		t.Fatal("GetHub() returned nil")
	}
	if hub.Client() == nil {
		t.Error("hub client should be configured")
	}
}

func TestLevelToInt(t *testing.T) {
	tests := []struct {
		in   bLogger.Level
		want string
	}{
		{bLogger.LevelDebug, "0"},
		{bLogger.LevelInfo, "1"},
		{bLogger.LevelWarn, "2"},
		{bLogger.LevelError, "3"},
	}
	for _, test := range tests {
		if got := levelToInt(test.in); got != test.want {
			t.Errorf("levelToInt(%v) = %q, want %q", test.in, got, test.want)
		}
	}
}
