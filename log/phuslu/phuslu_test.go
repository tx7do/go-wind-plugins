package phuslu

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	phlog "github.com/phuslu/log"

	bLogger "github.com/tx7do/go-wind/log"
)

// newTestLogger builds a phuslu-backed Logger writing JSON to an in-memory
// buffer at the given level.
func newTestLogger(level phlog.Level) (*Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	pl := &phlog.Logger{
		Level:  level,
		Writer: &phlog.IOWriter{Writer: buf},
	}
	return NewLoggerWith(pl), buf
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
			want: []string{`"level":"debug"`, `"message":"hello"`, `"k":"v"`},
		},
		"info": {
			call: func(l bLogger.Logger) { l.Info(context.Background(), "hello", "k", "v") },
			want: []string{`"level":"info"`, `"message":"hello"`, `"k":"v"`},
		},
		"warn": {
			call: func(l bLogger.Logger) { l.Warn(context.Background(), "hello", "k", "v") },
			want: []string{`"level":"warn"`, `"message":"hello"`, `"k":"v"`},
		},
		"error": {
			call: func(l bLogger.Logger) { l.Error(context.Background(), "hello", "k", "v") },
			want: []string{`"level":"error"`, `"message":"hello"`, `"k":"v"`},
		},
		"integer value": {
			call: func(l bLogger.Logger) { l.Info(context.Background(), "hello", "count", 42) },
			want: []string{`"count":42`},
		},
		"bool value": {
			call: func(l bLogger.Logger) { l.Info(context.Background(), "hello", "ok", true) },
			want: []string{`"ok":true`},
		},
		"no keyvals": {
			call: func(l bLogger.Logger) { l.Info(context.Background(), "plain message") },
			want: []string{`"level":"info"`, `"message":"plain message"`},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			logger, buf := newTestLogger(phlog.DebugLevel)
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
	logger, buf := newTestLogger(phlog.DebugLevel)
	logger.Info(context.Background(), "serving", "port", 8080, "env", "prod")

	out := buf.String()
	for _, want := range []string{`"port":8080`, `"env":"prod"`} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q does not contain %q", out, want)
		}
	}
}

// Odd trailing keyvals are silently dropped by the adapter.
func TestLogger_OddKeyvals(t *testing.T) {
	logger, buf := newTestLogger(phlog.DebugLevel)
	logger.Info(context.Background(), "hello", "lonely-key")

	out := buf.String()
	if !strings.Contains(out, `"message":"hello"`) {
		t.Errorf("output %q should still contain the message", out)
	}
	if strings.Contains(out, "lonely-key") {
		t.Errorf("output %q should not contain the unpaired key", out)
	}
}

// ---------------------------------------------------------------------------
// Level filtering
// ---------------------------------------------------------------------------

func TestLogger_LevelFiltering(t *testing.T) {
	logger, buf := newTestLogger(phlog.InfoLevel)

	logger.Debug(context.Background(), "hidden-debug")
	if buf.Len() != 0 {
		t.Errorf("expected no output for debug below Info level, got %q", buf.String())
	}

	logger.Info(context.Background(), "shown-info")
	logger.Warn(context.Background(), "shown-warn")
	logger.Error(context.Background(), "shown-error")

	out := buf.String()
	for _, want := range []string{`"level":"info"`, `"level":"warn"`, `"level":"error"`} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q does not contain %q", out, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Enabled
// ---------------------------------------------------------------------------

func TestLogger_Enabled(t *testing.T) {
	tests := []struct {
		loggerLevel phlog.Level
		level       bLogger.Level
		want        bool
	}{
		{phlog.InfoLevel, bLogger.LevelDebug, false},
		{phlog.InfoLevel, bLogger.LevelInfo, true},
		{phlog.InfoLevel, bLogger.LevelWarn, true},
		{phlog.InfoLevel, bLogger.LevelError, true},
		{phlog.DebugLevel, bLogger.LevelDebug, true},
		{phlog.ErrorLevel, bLogger.LevelWarn, false},
		{phlog.ErrorLevel, bLogger.LevelError, true},
		// Unknown levels map to Info.
		{phlog.InfoLevel, bLogger.Level(99), true},
		{phlog.DebugLevel, bLogger.Level(99), true},
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
// levelToPhuslu mapping
// ---------------------------------------------------------------------------

func TestLevelToPhuslu(t *testing.T) {
	tests := []struct {
		in   bLogger.Level
		want phlog.Level
	}{
		{bLogger.LevelDebug, phlog.DebugLevel},
		{bLogger.LevelInfo, phlog.InfoLevel},
		{bLogger.LevelWarn, phlog.WarnLevel},
		{bLogger.LevelError, phlog.ErrorLevel},
		{bLogger.Level(42), phlog.InfoLevel}, // unknown maps to Info
	}

	for _, test := range tests {
		if got := levelToPhuslu(test.in); got != test.want {
			t.Errorf("levelToPhuslu(%v) = %v, want %v", test.in, got, test.want)
		}
	}
}

// ---------------------------------------------------------------------------
// With — persistent keyvals
// ---------------------------------------------------------------------------

func TestLogger_With(t *testing.T) {
	logger, buf := newTestLogger(phlog.DebugLevel)
	child := logger.With("module", "http")

	child.Info(context.Background(), "served", "status", 200)

	out := buf.String()
	for _, want := range []string{`"module":"http"`, `"status":200`, `"message":"served"`} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q does not contain %q", out, want)
		}
	}
}

func TestLogger_With_Chained(t *testing.T) {
	logger, buf := newTestLogger(phlog.DebugLevel)
	child := logger.With("a", "1").With("b", "2")
	child.Info(context.Background(), "msg")

	out := buf.String()
	for _, want := range []string{`"a":"1"`, `"b":"2"`} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q does not contain %q", out, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Constructors, Close
// ---------------------------------------------------------------------------

func TestNewLogger(t *testing.T) {
	logger := NewLogger()
	if logger == nil {
		t.Fatal("NewLogger() returned nil")
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

// closeableWriter wraps an IOWriter and records Close calls so the adapter's
// Close delegation can be verified.
type closeableWriter struct {
	*phlog.IOWriter
	closed bool
	err    error
}

func (w *closeableWriter) Close() error {
	w.closed = true
	return w.err
}

func TestLogger_Close_DelegatesToWriter(t *testing.T) {
	buf := &bytes.Buffer{}
	w := &closeableWriter{IOWriter: &phlog.IOWriter{Writer: buf}}
	logger := NewLoggerWith(&phlog.Logger{
		Level:  phlog.DebugLevel,
		Writer: w,
	})

	if err := logger.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
	if !w.closed {
		t.Error("Close() should be delegated to the underlying writer")
	}
}

func TestLogger_Close_DelegatesWriterError(t *testing.T) {
	wantErr := errors.New("close failed")
	w := &closeableWriter{IOWriter: &phlog.IOWriter{Writer: &bytes.Buffer{}}, err: wantErr}
	logger := NewLoggerWith(&phlog.Logger{
		Level:  phlog.DebugLevel,
		Writer: w,
	})

	if err := logger.Close(); !errors.Is(err, wantErr) {
		t.Errorf("Close() error = %v, want %v", err, wantErr)
	}
}

func TestLogger_Close_NoCloserWriter(t *testing.T) {
	logger, _ := newTestLogger(phlog.DebugLevel)
	if err := logger.Close(); err != nil {
		t.Errorf("Close() with plain IOWriter error = %v, want nil", err)
	}
}
