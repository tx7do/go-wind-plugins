package charm

import (
	"bytes"
	"context"
	"strings"
	"testing"

	clog "github.com/charmbracelet/log"

	bLogger "github.com/tx7do/go-wind/log"
)

// newTestLogger builds a charm-backed Logger writing to an in-memory buffer
// with deterministic formatting (no timestamps, no caller info).
func newTestLogger(level clog.Level) (*Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	cl := clog.New(buf)
	cl.SetLevel(level)
	cl.SetReportTimestamp(false)
	cl.SetReportCaller(false)
	return NewLoggerWith(cl), buf
}

// ---------------------------------------------------------------------------
// Level emission and keyval formatting
// ---------------------------------------------------------------------------

func TestLoggerLevels(t *testing.T) {
	tests := map[string]struct {
		call func(l bLogger.Logger)
		want []string
	}{
		"debug": {
			call: func(l bLogger.Logger) { l.Debug(context.Background(), "hello", "k", "v") },
			want: []string{"DEBU hello", "k=v"},
		},
		"info": {
			call: func(l bLogger.Logger) { l.Info(context.Background(), "hello", "k", "v") },
			want: []string{"INFO hello", "k=v"},
		},
		"warn": {
			call: func(l bLogger.Logger) { l.Warn(context.Background(), "hello", "k", "v") },
			want: []string{"WARN hello", "k=v"},
		},
		"error": {
			call: func(l bLogger.Logger) { l.Error(context.Background(), "hello", "k", "v") },
			want: []string{"ERRO hello", "k=v"},
		},
		"string value with space is quoted": {
			call: func(l bLogger.Logger) { l.Info(context.Background(), "hello", "phrase", "two words") },
			want: []string{"INFO hello", `phrase="two words"`},
		},
		"integer value": {
			call: func(l bLogger.Logger) { l.Info(context.Background(), "hello", "count", 42) },
			want: []string{"INFO hello", "count=42"},
		},
		"no keyvals": {
			call: func(l bLogger.Logger) { l.Info(context.Background(), "plain message") },
			want: []string{"INFO plain message"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			logger, buf := newTestLogger(clog.DebugLevel)
			test.call(logger)

			out := buf.String()
			if out == "" {
				t.Fatal("expected log output, got empty buffer")
			}
			for _, w := range test.want {
				if !strings.Contains(out, w) {
					t.Errorf("output %q does not contain %q", out, w)
				}
			}
		})
	}
}

func TestLogger_MultipleKeyvals(t *testing.T) {
	logger, buf := newTestLogger(clog.DebugLevel)
	logger.Info(context.Background(), "serving", "port", 8080, "env", "prod")

	out := buf.String()
	for _, want := range []string{"INFO serving", "port=8080", "env=prod"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q does not contain %q", out, want)
		}
	}
}

// Odd trailing keyvals must not panic; the message itself is still emitted.
func TestLogger_OddKeyvals(t *testing.T) {
	logger, buf := newTestLogger(clog.DebugLevel)
	logger.Info(context.Background(), "hello", "lonely-key")

	if out := buf.String(); !strings.Contains(out, "hello") {
		t.Errorf("output %q should still contain the message", out)
	}
}

// ---------------------------------------------------------------------------
// Level filtering
// ---------------------------------------------------------------------------

func TestLogger_LevelFiltering(t *testing.T) {
	logger, buf := newTestLogger(clog.WarnLevel)

	logger.Debug(context.Background(), "hidden-debug")
	logger.Info(context.Background(), "hidden-info")
	if buf.Len() != 0 {
		t.Errorf("expected no output below Warn level, got %q", buf.String())
	}

	logger.Warn(context.Background(), "shown-warn")
	if !strings.Contains(buf.String(), "WARN shown-warn") {
		t.Errorf("output %q should contain the warn record", buf.String())
	}

	logger.Error(context.Background(), "shown-error")
	if !strings.Contains(buf.String(), "ERRO shown-error") {
		t.Errorf("output %q should contain the error record", buf.String())
	}
}

// ---------------------------------------------------------------------------
// Enabled
// ---------------------------------------------------------------------------

func TestLogger_Enabled(t *testing.T) {
	tests := []struct {
		loggerLevel clog.Level
		level       bLogger.Level
		want        bool
	}{
		{clog.InfoLevel, bLogger.LevelDebug, false},
		{clog.InfoLevel, bLogger.LevelInfo, true},
		{clog.InfoLevel, bLogger.LevelWarn, true},
		{clog.InfoLevel, bLogger.LevelError, true},
		{clog.DebugLevel, bLogger.LevelDebug, true},
		{clog.ErrorLevel, bLogger.LevelWarn, false},
		{clog.ErrorLevel, bLogger.LevelError, true},
		// Unknown levels map to Info.
		{clog.InfoLevel, bLogger.Level(99), true},
		{clog.DebugLevel, bLogger.Level(99), true},
	}

	for _, test := range tests {
		logger, _ := newTestLogger(test.loggerLevel)
		if got := logger.Enabled(test.level); got != test.want {
			t.Errorf("Enabled(%v) with logger level %v = %v, want %v",
				test.level, test.loggerLevel, got, test.want)
		}
	}
}

// ---------------------------------------------------------------------------
// levelToCharm mapping
// ---------------------------------------------------------------------------

func TestLevelToCharm(t *testing.T) {
	tests := []struct {
		in   bLogger.Level
		want clog.Level
	}{
		{bLogger.LevelDebug, clog.DebugLevel},
		{bLogger.LevelInfo, clog.InfoLevel},
		{bLogger.LevelWarn, clog.WarnLevel},
		{bLogger.LevelError, clog.ErrorLevel},
		{bLogger.Level(42), clog.InfoLevel}, // unknown maps to Info
	}

	for _, test := range tests {
		if got := levelToCharm(test.in); got != test.want {
			t.Errorf("levelToCharm(%v) = %v, want %v", test.in, got, test.want)
		}
	}
}

// ---------------------------------------------------------------------------
// With
// ---------------------------------------------------------------------------

func TestLogger_With(t *testing.T) {
	logger, buf := newTestLogger(clog.DebugLevel)
	child := logger.With("module", "http")

	childLogger, ok := child.(*Logger)
	if !ok {
		t.Fatalf("With() returned %T, want *Logger", child)
	}
	childLogger.Info(context.Background(), "served", "status", 200)

	out := buf.String()
	for _, want := range []string{"INFO served", "module=http", "status=200"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q does not contain %q", out, want)
		}
	}
}

func TestLogger_With_Chained(t *testing.T) {
	logger, buf := newTestLogger(clog.DebugLevel)
	child := logger.With("a", "1").With("b", "2")
	child.Info(context.Background(), "msg")

	out := buf.String()
	if !strings.Contains(out, "a=1") || !strings.Contains(out, "b=2") {
		t.Errorf("output %q should contain both chained keyvals", out)
	}
}

// ---------------------------------------------------------------------------
// Constructors, Close, String
// ---------------------------------------------------------------------------

func TestNewLogger(t *testing.T) {
	logger := NewLogger()
	if logger == nil {
		t.Fatal("NewLogger() returned nil")
	}
	// Default level is Info: debug disabled, error enabled.
	if logger.Enabled(bLogger.LevelDebug) {
		t.Error("default logger should not enable debug")
	}
	if !logger.Enabled(bLogger.LevelError) {
		t.Error("default logger should enable error")
	}
}

func TestNewLoggerWith_NilFallsBackToDefault(t *testing.T) {
	logger := NewLoggerWith(nil)
	if logger == nil {
		t.Fatal("NewLoggerWith(nil) returned nil")
	}
	if logger.Enabled(bLogger.LevelDebug) {
		t.Error("fallback logger should use default Info level")
	}
	if !logger.Enabled(bLogger.LevelInfo) {
		t.Error("fallback logger should enable info")
	}
}

func TestLogger_Close(t *testing.T) {
	logger, _ := newTestLogger(clog.DebugLevel)
	if err := logger.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

func TestLogger_String(t *testing.T) {
	logger, _ := newTestLogger(clog.DebugLevel)
	s := logger.String()
	if !strings.Contains(s, "charm.Logger{level=") {
		t.Errorf("String() = %q, want prefix \"charm.Logger{level=\"", s)
	}
}
