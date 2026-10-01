package hclog

import (
	"bytes"
	"context"
	"strings"
	"testing"

	hclog "github.com/hashicorp/go-hclog"

	bLogger "github.com/tx7do/go-wind/log"
)

// newTestLogger builds an hclog-backed Logger writing JSON (or plain text)
// to an in-memory buffer with timestamps disabled for deterministic output.
func newTestLogger(level hclog.Level, jsonFormat bool) (*Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	l := hclog.New(&hclog.LoggerOptions{
		Name:        "test",
		Level:       level,
		Output:      buf,
		JSONFormat:  jsonFormat,
		DisableTime: true,
	})
	return NewLoggerWith(l), buf
}

// ---------------------------------------------------------------------------
// Level emission and keyval formatting (JSON output)
// ---------------------------------------------------------------------------

func TestLoggerLevels_JSON(t *testing.T) {
	tests := map[string]struct {
		call func(l bLogger.Logger)
		want []string
	}{
		"debug": {
			call: func(l bLogger.Logger) { l.Debug(context.Background(), "hello", "k", "v") },
			want: []string{`"@level":"debug"`, `"@message":"hello"`, `"@module":"test"`, `"k":"v"`},
		},
		"info": {
			call: func(l bLogger.Logger) { l.Info(context.Background(), "hello", "k", "v") },
			want: []string{`"@level":"info"`, `"@message":"hello"`, `"k":"v"`},
		},
		"warn": {
			call: func(l bLogger.Logger) { l.Warn(context.Background(), "hello", "k", "v") },
			want: []string{`"@level":"warn"`, `"@message":"hello"`, `"k":"v"`},
		},
		"error": {
			call: func(l bLogger.Logger) { l.Error(context.Background(), "hello", "k", "v") },
			want: []string{`"@level":"error"`, `"@message":"hello"`, `"k":"v"`},
		},
		"integer value": {
			call: func(l bLogger.Logger) { l.Info(context.Background(), "hello", "count", 42) },
			want: []string{`"count":42`},
		},
		"no keyvals": {
			call: func(l bLogger.Logger) { l.Info(context.Background(), "plain message") },
			want: []string{`"@level":"info"`, `"@message":"plain message"`},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			logger, buf := newTestLogger(hclog.Debug, true)
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

func TestLoggerLevels_PlainText(t *testing.T) {
	tests := map[string]struct {
		call func(l bLogger.Logger)
		want []string
	}{
		"debug": {
			call: func(l bLogger.Logger) { l.Debug(context.Background(), "hello", "k", "v") },
			want: []string{"[DEBUG]", "hello", "k=v"},
		},
		"info": {
			call: func(l bLogger.Logger) { l.Info(context.Background(), "hello", "k", "v") },
			want: []string{"[INFO]", "hello", "k=v"},
		},
		"warn": {
			call: func(l bLogger.Logger) { l.Warn(context.Background(), "hello", "k", "v") },
			want: []string{"[WARN]", "hello", "k=v"},
		},
		"error": {
			call: func(l bLogger.Logger) { l.Error(context.Background(), "hello", "k", "v") },
			want: []string{"[ERROR]", "hello", "k=v"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			logger, buf := newTestLogger(hclog.Debug, false)
			test.call(logger)

			out := buf.String()
			for _, w := range test.want {
				if !strings.Contains(out, w) {
					t.Errorf("output %q does not contain %q", out, w)
				}
			}
		})
	}
}

func TestLogger_MultipleKeyvals(t *testing.T) {
	logger, buf := newTestLogger(hclog.Debug, true)
	logger.Info(context.Background(), "serving", "port", 8080, "env", "prod")

	out := buf.String()
	for _, want := range []string{`"port":8080`, `"env":"prod"`} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q does not contain %q", out, want)
		}
	}
}

// Odd trailing keyvals must not panic; hclog emits its own warning but the
// message itself is still recorded.
func TestLogger_OddKeyvals(t *testing.T) {
	logger, buf := newTestLogger(hclog.Debug, true)
	logger.Info(context.Background(), "hello", "lonely-key")

	if out := buf.String(); !strings.Contains(out, `"@message":"hello"`) {
		t.Errorf("output %q should still contain the message", out)
	}
}

// ---------------------------------------------------------------------------
// Level filtering
// ---------------------------------------------------------------------------

func TestLogger_LevelFiltering(t *testing.T) {
	logger, buf := newTestLogger(hclog.Warn, true)

	logger.Debug(context.Background(), "hidden-debug")
	logger.Info(context.Background(), "hidden-info")
	if buf.Len() != 0 {
		t.Errorf("expected no output below Warn level, got %q", buf.String())
	}

	logger.Warn(context.Background(), "shown-warn")
	logger.Error(context.Background(), "shown-error")

	out := buf.String()
	if !strings.Contains(out, `"@level":"warn"`) || !strings.Contains(out, `"@level":"error"`) {
		t.Errorf("output %q should contain warn and error records", out)
	}
}

// ---------------------------------------------------------------------------
// Enabled
// ---------------------------------------------------------------------------

func TestLogger_Enabled(t *testing.T) {
	tests := []struct {
		loggerLevel hclog.Level
		level       bLogger.Level
		want        bool
	}{
		{hclog.Info, bLogger.LevelDebug, false},
		{hclog.Info, bLogger.LevelInfo, true},
		{hclog.Info, bLogger.LevelWarn, true},
		{hclog.Info, bLogger.LevelError, true},
		{hclog.Debug, bLogger.LevelDebug, true},
		{hclog.Error, bLogger.LevelWarn, false},
		{hclog.Error, bLogger.LevelError, true},
		// Unknown levels map to Info.
		{hclog.Info, bLogger.Level(99), true},
		{hclog.Debug, bLogger.Level(99), true},
	}

	for _, test := range tests {
		logger, _ := newTestLogger(test.loggerLevel, true)
		if got := logger.Enabled(test.level); got != test.want {
			t.Errorf("Enabled(%v) with logger level %v = %v, want %v",
				test.level, test.loggerLevel, got, test.want)
		}
	}
}

// ---------------------------------------------------------------------------
// levelToHclog mapping
// ---------------------------------------------------------------------------

func TestLevelToHclog(t *testing.T) {
	tests := []struct {
		in   bLogger.Level
		want hclog.Level
	}{
		{bLogger.LevelDebug, hclog.Debug},
		{bLogger.LevelInfo, hclog.Info},
		{bLogger.LevelWarn, hclog.Warn},
		{bLogger.LevelError, hclog.Error},
		{bLogger.Level(42), hclog.Info}, // unknown maps to Info
	}

	for _, test := range tests {
		if got := levelToHclog(test.in); got != test.want {
			t.Errorf("levelToHclog(%v) = %v, want %v", test.in, got, test.want)
		}
	}
}

// ---------------------------------------------------------------------------
// With / Named
// ---------------------------------------------------------------------------

func TestLogger_With(t *testing.T) {
	logger, buf := newTestLogger(hclog.Debug, true)
	child := logger.With("module", "vault")

	child.Info(context.Background(), "unsealing")

	out := buf.String()
	if !strings.Contains(out, `"module":"vault"`) {
		t.Errorf("output %q does not contain the With keyval", out)
	}
	if !strings.Contains(out, `"@message":"unsealing"`) {
		t.Errorf("output %q does not contain the message", out)
	}
}

func TestLogger_Named(t *testing.T) {
	logger, buf := newTestLogger(hclog.Debug, true)
	child, ok := logger.Named("sub").(*Logger)
	if !ok {
		t.Fatalf("Named() returned %T, want *Logger", logger.Named("sub"))
	}

	if got := child.Name(); got != "test.sub" {
		t.Errorf("Named().Name() = %q, want %q", got, "test.sub")
	}

	child.Info(context.Background(), "child message")
	if out := buf.String(); !strings.Contains(out, `"@module":"test.sub"`) {
		t.Errorf("output %q does not contain the child module name", out)
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
	if got := logger.Name(); got != "app" {
		t.Errorf("default logger Name() = %q, want %q", got, "app")
	}
	// Default level is Info.
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
	logger, _ := newTestLogger(hclog.Debug, true)
	if err := logger.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

func TestLogger_String(t *testing.T) {
	logger, _ := newTestLogger(hclog.Debug, true)
	if got := logger.String(); got != "hclog.Logger{name=test}" {
		t.Errorf("String() = %q, want %q", got, "hclog.Logger{name=test}")
	}
}
