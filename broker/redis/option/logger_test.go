package option

import (
	"context"
	"testing"

	"github.com/tx7do/go-wind/log"
)

// mockLogger records the log calls it receives.
type mockLogger struct {
	levels []log.Level
	msgs   []string
}

func (m *mockLogger) Debug(_ context.Context, msg string, _ ...any) { m.record(log.LevelDebug, msg) }
func (m *mockLogger) Info(_ context.Context, msg string, _ ...any)  { m.record(log.LevelInfo, msg) }
func (m *mockLogger) Warn(_ context.Context, msg string, _ ...any)  { m.record(log.LevelWarn, msg) }
func (m *mockLogger) Error(_ context.Context, msg string, _ ...any) { m.record(log.LevelError, msg) }
func (m *mockLogger) Enabled(_ log.Level) bool                      { return true }
func (m *mockLogger) With(_ ...any) log.Logger                      { return m }

func (m *mockLogger) record(l log.Level, msg string) {
	m.levels = append(m.levels, l)
	m.msgs = append(m.msgs, msg)
}

func (m *mockLogger) count() int { return len(m.msgs) }

// ---------------------------------------------------------------------------
// SetLogger / getLogger
// ---------------------------------------------------------------------------

func TestSetLogger_RoutesLogCalls(t *testing.T) {
	ml := &mockLogger{}
	SetLogger(ml)
	defer SetLogger(nil)

	LogDebug("debug msg")
	LogInfo("info msg")
	LogWarn("warn msg")
	LogError("error msg")

	if ml.count() != 4 {
		t.Fatalf("logger received %d calls, want 4 (msgs=%v)", ml.count(), ml.msgs)
	}

	wantLevels := []log.Level{log.LevelDebug, log.LevelInfo, log.LevelWarn, log.LevelError}
	for i, want := range wantLevels {
		if ml.levels[i] != want {
			t.Errorf("call %d level = %v, want %v", i, ml.levels[i], want)
		}
	}

	// Every message must carry the package log key prefix.
	for i, msg := range ml.msgs {
		if len(msg) < 7 || msg[:7] != "[redis]" {
			t.Errorf("call %d msg = %q, want \"[redis] \" prefix", i, msg)
		}
	}
}

func TestSetLogger_NilRestoresGlobal(t *testing.T) {
	ml := &mockLogger{}
	SetLogger(ml)

	// Passing nil must restore the framework global logger, so the mock
	// stops receiving messages.
	SetLogger(nil)
	LogInfo("after reset")

	if ml.count() != 0 {
		t.Errorf("mock logger received %d calls after SetLogger(nil), want 0", ml.count())
	}
}

func TestSetLogger_Overwrite(t *testing.T) {
	first := &mockLogger{}
	second := &mockLogger{}

	SetLogger(first)
	SetLogger(second)
	defer SetLogger(nil)

	LogInfo("to second")

	if first.count() != 0 {
		t.Errorf("first logger received %d calls, want 0 after overwrite", first.count())
	}
	if second.count() != 1 {
		t.Errorf("second logger received %d calls, want 1", second.count())
	}
}

// ---------------------------------------------------------------------------
// Log helpers
// ---------------------------------------------------------------------------

func TestLogfHelpers_Format(t *testing.T) {
	ml := &mockLogger{}
	SetLogger(ml)
	defer SetLogger(nil)

	LogDebugf("count=%d", 1)
	LogInfof("count=%d", 2)
	LogWarnf("count=%d", 3)
	LogErrorf("count=%d", 4)

	wantMsgs := []string{"[redis] count=1", "[redis] count=2", "[redis] count=3", "[redis] count=4"}
	if ml.count() != len(wantMsgs) {
		t.Fatalf("logger received %d calls, want %d (msgs=%v)", ml.count(), len(wantMsgs), ml.msgs)
	}
	for i, want := range wantMsgs {
		if ml.msgs[i] != want {
			t.Errorf("call %d msg = %q, want %q", i, ml.msgs[i], want)
		}
	}
}

func TestLogFatal_LogsAtErrorLevelWithoutExiting(t *testing.T) {
	ml := &mockLogger{}
	SetLogger(ml)
	defer SetLogger(nil)

	// LogFatal/LogFatalf must map to the error level (not os.Exit).
	LogFatal("fatal msg")
	LogFatalf("fatal %s", "formatted")

	if ml.count() != 2 {
		t.Fatalf("logger received %d calls, want 2", ml.count())
	}
	for i, lvl := range ml.levels {
		if lvl != log.LevelError {
			t.Errorf("call %d level = %v, want %v", i, lvl, log.LevelError)
		}
	}
}

func TestLogAt_UnknownLevelFallsBackToError(t *testing.T) {
	ml := &mockLogger{}
	SetLogger(ml)
	defer SetLogger(nil)

	// An unmapped level must route to the error logger.
	logAt(log.Level(99), "fallback")

	if ml.count() != 1 {
		t.Fatalf("logger received %d calls, want 1", ml.count())
	}
	if ml.levels[0] != log.LevelError {
		t.Errorf("level = %v, want %v", ml.levels[0], log.LevelError)
	}
	if ml.msgs[0] != "[redis] fallback" {
		t.Errorf("msg = %q, want %q", ml.msgs[0], "[redis] fallback")
	}
}
