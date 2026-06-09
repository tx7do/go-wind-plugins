package log

import (
	"context"
	"sync"
	"testing"

	windlog "github.com/tx7do/go-wind/log"
)

// ---------------------------------------------------------------------------
// mockLogger — records calls for verification
// ---------------------------------------------------------------------------

type mockLogger struct {
	mu     sync.Mutex
	calls  []string
	fields []any
	// levelToReturn controls what Enabled() reports
	levelToReturn windlog.Level
}

func (m *mockLogger) Debug(_ context.Context, msg string, _ ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "debug:"+msg)
}

func (m *mockLogger) Info(_ context.Context, msg string, _ ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "info:"+msg)
}

func (m *mockLogger) Warn(_ context.Context, msg string, _ ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "warn:"+msg)
}

func (m *mockLogger) Error(_ context.Context, msg string, _ ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "error:"+msg)
}

func (m *mockLogger) Enabled(level windlog.Level) bool {
	return level >= m.levelToReturn
}

func (m *mockLogger) With(keyvals ...any) windlog.Logger {
	return &mockLogger{
		fields:        append(append([]any{}, m.fields...), keyvals...),
		levelToReturn: m.levelToReturn,
	}
}

func (m *mockLogger) getCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]string, len(m.calls))
	copy(copied, m.calls)
	return copied
}

var _ windlog.Logger = (*mockLogger)(nil)

// ---------------------------------------------------------------------------
// MultiLogger
// ---------------------------------------------------------------------------

func TestMultiLogger_FansOutToAll(t *testing.T) {
	a := &mockLogger{}
	b := &mockLogger{}
	ml := MultiLogger{Loggers: []windlog.Logger{a, b}}

	ml.Info(context.Background(), "hello")
	ml.Error(context.Background(), "world")

	aCalls := a.getCalls()
	bCalls := b.getCalls()

	if len(aCalls) != 2 {
		t.Errorf("logger A received %d calls, want 2", len(aCalls))
	}
	if len(bCalls) != 2 {
		t.Errorf("logger B received %d calls, want 2", len(bCalls))
	}
}

func TestMultiLogger_FourLevels(t *testing.T) {
	m := &mockLogger{}
	ml := MultiLogger{Loggers: []windlog.Logger{m}}

	ml.Debug(context.Background(), "d")
	ml.Info(context.Background(), "i")
	ml.Warn(context.Background(), "w")
	ml.Error(context.Background(), "e")

	calls := m.getCalls()
	if len(calls) != 4 {
		t.Fatalf("expected 4 calls, got %d", len(calls))
	}
	want := []string{"debug:d", "info:i", "warn:w", "error:e"}
	for i, w := range want {
		if calls[i] != w {
			t.Errorf("call[%d] = %q, want %q", i, calls[i], w)
		}
	}
}

func TestMultiLogger_EnabledAnyTrue(t *testing.T) {
	// One logger only accepts Error, one accepts everything.
	// MultiLogger.Enabled should be true if ANY logger reports enabled.
	a := &mockLogger{levelToReturn: windlog.LevelError} // only Error and above
	b := &mockLogger{levelToReturn: windlog.LevelDebug} // all levels

	ml := MultiLogger{Loggers: []windlog.Logger{a, b}}

	if !ml.Enabled(windlog.LevelDebug) {
		t.Error("Enabled(Debug) should be true because logger B accepts all levels")
	}
}

func TestMultiLogger_EnabledNoneFalse(t *testing.T) {
	// Both loggers only accept Error, so Debug should not be enabled.
	a := &mockLogger{levelToReturn: windlog.LevelError}
	b := &mockLogger{levelToReturn: windlog.LevelError}

	ml := MultiLogger{Loggers: []windlog.Logger{a, b}}

	if ml.Enabled(windlog.LevelDebug) {
		t.Error("Enabled(Debug) should be false when all loggers require Error")
	}
	if !ml.Enabled(windlog.LevelError) {
		t.Error("Enabled(Error) should be true")
	}
}

func TestMultiLogger_With(t *testing.T) {
	a := &mockLogger{}
	b := &mockLogger{}
	ml := MultiLogger{Loggers: []windlog.Logger{a, b}}

	child := ml.With("module", "test")
	if childML, ok := child.(MultiLogger); ok {
		if len(childML.Loggers) != 2 {
			t.Errorf("child MultiLogger has %d loggers, want 2", len(childML.Loggers))
		}
	} else {
		t.Errorf("With() should return a MultiLogger, got %T", child)
	}

	// Verify child still works
	child.Info(context.Background(), "from child")
}

func TestMultiLogger_Empty(t *testing.T) {
	ml := MultiLogger{Loggers: []windlog.Logger{}}
	// Should not panic
	ml.Info(context.Background(), "noop")
	if ml.Enabled(windlog.LevelDebug) {
		t.Error("empty MultiLogger should not be enabled")
	}
}

func TestMultiLogger_PanicRecovery(t *testing.T) {
	panicLogger := &panickingMock{}
	safeLogger := &mockLogger{}

	ml := MultiLogger{Loggers: []windlog.Logger{panicLogger, safeLogger}}

	// Should not panic — the fan-out recovers
	ml.Info(context.Background(), "test")

	calls := safeLogger.getCalls()
	if len(calls) != 1 {
		t.Errorf("safe logger should still receive the call, got %d", len(calls))
	}
}

type panickingMock struct{}

func (panickingMock) Debug(context.Context, string, ...any) { panic("debug boom") }
func (panickingMock) Info(context.Context, string, ...any)  { panic("info boom") }
func (panickingMock) Warn(context.Context, string, ...any)  { panic("warn boom") }
func (panickingMock) Error(context.Context, string, ...any) { panic("error boom") }
func (panickingMock) Enabled(windlog.Level) bool            { return true }
func (p panickingMock) With(...any) windlog.Logger          { return p }

var _ windlog.Logger = panickingMock{}

// ---------------------------------------------------------------------------
// LevelFilter
// ---------------------------------------------------------------------------

func TestLevelFilter_DiscardsBelowThreshold(t *testing.T) {
	mock := &mockLogger{levelToReturn: windlog.LevelDebug}
	filter := LevelFilter{Logger: mock, Level: windlog.LevelWarn}

	filter.Debug(context.Background(), "debug-msg")
	filter.Info(context.Background(), "info-msg")

	calls := mock.getCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 calls below Warn threshold, got %d: %v", len(calls), calls)
	}
}

func TestLevelFilter_PassesAtOrAboveThreshold(t *testing.T) {
	mock := &mockLogger{levelToReturn: windlog.LevelDebug} // accepts all levels
	filter := LevelFilter{Logger: mock, Level: windlog.LevelWarn}

	filter.Warn(context.Background(), "warn-msg")
	filter.Error(context.Background(), "error-msg")

	calls := mock.getCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls at/above Warn, got %d", len(calls))
	}
	if calls[0] != "warn:warn-msg" {
		t.Errorf("call[0] = %q, want %q", calls[0], "warn:warn-msg")
	}
	if calls[1] != "error:error-msg" {
		t.Errorf("call[1] = %q, want %q", calls[1], "error:error-msg")
	}
}

func TestLevelFilter_Enabled(t *testing.T) {
	mock := &mockLogger{levelToReturn: windlog.LevelDebug}
	filter := LevelFilter{Logger: mock, Level: windlog.LevelWarn}

	if filter.Enabled(windlog.LevelDebug) {
		t.Error("Enabled(Debug) should be false when threshold is Warn")
	}
	if !filter.Enabled(windlog.LevelWarn) {
		t.Error("Enabled(Warn) should be true")
	}
	if !filter.Enabled(windlog.LevelError) {
		t.Error("Enabled(Error) should be true")
	}
}

func TestLevelFilter_With(t *testing.T) {
	mock := &mockLogger{levelToReturn: windlog.LevelDebug}
	filter := LevelFilter{Logger: mock, Level: windlog.LevelWarn}

	child := filter.With("key", "val")
	if childLF, ok := child.(LevelFilter); ok {
		if childLF.Level != windlog.LevelWarn {
			t.Errorf("child Level = %v, want %v", childLF.Level, windlog.LevelWarn)
		}
	} else {
		t.Errorf("With() should return a LevelFilter, got %T", child)
	}

	// Child should still filter correctly
	child.Debug(context.Background(), "hidden")
	child.Error(context.Background(), "visible")
}

func TestLevelFilter_AllowsAllAtDebugLevel(t *testing.T) {
	mock := &mockLogger{levelToReturn: windlog.LevelDebug} // accepts all levels
	filter := LevelFilter{Logger: mock, Level: windlog.LevelDebug}

	filter.Debug(context.Background(), "d")
	filter.Info(context.Background(), "i")
	filter.Warn(context.Background(), "w")
	filter.Error(context.Background(), "e")

	calls := mock.getCalls()
	if len(calls) != 4 {
		t.Errorf("expected 4 calls with Debug threshold, got %d", len(calls))
	}
}

// ---------------------------------------------------------------------------
// SlogLogger — basic smoke test
// ---------------------------------------------------------------------------

func TestSlogLogger_BasicUsage(t *testing.T) {
	logger := NewSlogLogger()
	if logger == nil {
		t.Fatal("NewSlogLogger() returned nil")
	}

	// Should not panic
	logger.Debug(context.Background(), "debug-msg")
	logger.Info(context.Background(), "info-msg", "key", "val")
	logger.Warn(context.Background(), "warn-msg")
	logger.Error(context.Background(), "error-msg")
}

func TestSlogLogger_WithNilContext(t *testing.T) {
	logger := NewSlogLogger()
	// ensureCtx should handle nil context without panic
	logger.Info(nil, "nil-ctx-msg")
}

func TestSlogLogger_Enabled(t *testing.T) {
	logger := NewSlogLogger().(SlogLogger)
	// SlogLogger at INFO level: Debug should be disabled, Info/Error enabled
	if logger.Enabled(windlog.LevelError) != true {
		t.Error("Enabled(Error) should be true at INFO level")
	}
}

func TestSlogLogger_With(t *testing.T) {
	logger := NewSlogLogger()
	child := logger.With("module", "test")
	if child == nil {
		t.Fatal("With() returned nil")
	}
	child.Info(context.Background(), "child-msg")
}
