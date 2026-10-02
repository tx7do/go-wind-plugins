package logrus

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	bLogger "github.com/tx7do/go-wind/log"
)

// newBufferLogger builds a logrus-backed Logger writing JSON to buf with
// deterministic formatting (no timestamps, no caller).
func newBufferLogger(level logrus.Level) (*Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	l := logrus.New()
	l.Level = level
	l.Out = buf
	l.Formatter = &logrus.JSONFormatter{DisableTimestamp: true}
	return NewLogrusLogger(l), buf
}

func decode(t *testing.T, line string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &m); err != nil {
		t.Fatalf("decode log line %q: %v", line, err)
	}
	return m
}

// ---------------------------------------------------------------------------
// Level emission and keyval formatting
// ---------------------------------------------------------------------------

func TestLoggerLevels(t *testing.T) {
	tests := map[string]struct {
		call func(l bLogger.Logger)
		want string
	}{
		"debug": {call: func(l bLogger.Logger) { l.Debug(context.Background(), "hello", "k", "v") }, want: "debug"},
		"info":  {call: func(l bLogger.Logger) { l.Info(context.Background(), "hello", "k", "v") }, want: "info"},
		"warn":  {call: func(l bLogger.Logger) { l.Warn(context.Background(), "hello", "k", "v") }, want: "warning"},
		"error": {call: func(l bLogger.Logger) { l.Error(context.Background(), "hello", "k", "v") }, want: "error"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			logger, buf := newBufferLogger(logrus.TraceLevel)
			tt.call(logger)

			out := buf.String()
			if out == "" {
				t.Fatal("expected log output, got empty buffer")
			}
			m := decode(t, out)
			if m["level"] != tt.want {
				t.Errorf("level = %v, want %v", m["level"], tt.want)
			}
			if m["msg"] != "hello" {
				t.Errorf("msg = %v, want hello", m["msg"])
			}
			if m["k"] != "v" {
				t.Errorf("k = %v, want v", m["k"])
			}
		})
	}
}

func TestLogger_MultipleAndTypedKeyvals(t *testing.T) {
	logger, buf := newBufferLogger(logrus.TraceLevel)
	logger.Info(context.Background(), "serving", "port", 8080, "debug", true)

	m := decode(t, buf.String())
	if m["port"] != float64(8080) {
		t.Errorf("port = %v, want 8080", m["port"])
	}
	if m["debug"] != true {
		t.Errorf("debug = %v, want true", m["debug"])
	}
}

// Odd trailing keyvals are dropped by toFields; the message still emits.
func TestLogger_OddKeyvals(t *testing.T) {
	logger, buf := newBufferLogger(logrus.TraceLevel)
	logger.Info(context.Background(), "hello", "lonely-key")

	m := decode(t, buf.String())
	if m["msg"] != "hello" {
		t.Errorf("msg = %v, want hello", m["msg"])
	}
	if _, ok := m["lonely-key"]; ok {
		t.Error("odd trailing key should be dropped")
	}
}

func TestLogger_NoKeyvals(t *testing.T) {
	logger, buf := newBufferLogger(logrus.TraceLevel)
	logger.Info(context.Background(), "plain message")

	m := decode(t, buf.String())
	if m["msg"] != "plain message" {
		t.Errorf("msg = %v, want %q", m["msg"], "plain message")
	}
	if _, ok := m["level"]; !ok {
		t.Error("level field missing")
	}
}

// ---------------------------------------------------------------------------
// Level filtering
// ---------------------------------------------------------------------------

func TestLogger_LevelFiltering(t *testing.T) {
	logger, buf := newBufferLogger(logrus.WarnLevel)

	logger.Debug(context.Background(), "hidden-debug")
	logger.Info(context.Background(), "hidden-info")
	if buf.Len() != 0 {
		t.Errorf("expected no output below Warn level, got %q", buf.String())
	}

	logger.Warn(context.Background(), "shown-warn")
	logger.Error(context.Background(), "shown-error")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 records, got %d: %q", len(lines), buf.String())
	}
	if m := decode(t, lines[0]); m["msg"] != "shown-warn" {
		t.Errorf("first record = %v, want shown-warn", m["msg"])
	}
	if m := decode(t, lines[1]); m["msg"] != "shown-error" {
		t.Errorf("second record = %v, want shown-error", m["msg"])
	}
}

// ---------------------------------------------------------------------------
// Enabled and level mapping
// ---------------------------------------------------------------------------

func TestLogger_Enabled(t *testing.T) {
	tests := []struct {
		loggerLevel logrus.Level
		level       bLogger.Level
		want        bool
	}{
		{logrus.InfoLevel, bLogger.LevelDebug, false},
		{logrus.InfoLevel, bLogger.LevelInfo, true},
		{logrus.InfoLevel, bLogger.LevelWarn, true},
		{logrus.InfoLevel, bLogger.LevelError, true},
		{logrus.DebugLevel, bLogger.LevelDebug, true},
		{logrus.ErrorLevel, bLogger.LevelWarn, false},
		{logrus.ErrorLevel, bLogger.LevelError, true},
		// Unknown levels map to Info.
		{logrus.InfoLevel, bLogger.Level(99), true},
		{logrus.DebugLevel, bLogger.Level(99), true},
		{logrus.ErrorLevel, bLogger.Level(99), false},
	}

	for _, tt := range tests {
		logger, _ := newBufferLogger(tt.loggerLevel)
		if got := logger.Enabled(tt.level); got != tt.want {
			t.Errorf("Enabled(%v) with logger level %v = %v, want %v",
				tt.level, tt.loggerLevel, got, tt.want)
		}
	}
}

func TestLevelToLogrus(t *testing.T) {
	tests := []struct {
		in   bLogger.Level
		want logrus.Level
	}{
		{bLogger.LevelDebug, logrus.DebugLevel},
		{bLogger.LevelInfo, logrus.InfoLevel},
		{bLogger.LevelWarn, logrus.WarnLevel},
		{bLogger.LevelError, logrus.ErrorLevel},
		{bLogger.Level(42), logrus.InfoLevel},
	}
	for _, tt := range tests {
		if got := levelToLogrus(tt.in); got != tt.want {
			t.Errorf("levelToLogrus(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// With
// ---------------------------------------------------------------------------

func TestLogger_With(t *testing.T) {
	logger, buf := newBufferLogger(logrus.TraceLevel)
	child := logger.With("module", "http")

	childLogger, ok := child.(*Logger)
	if !ok {
		t.Fatalf("With() returned %T, want *Logger", child)
	}
	childLogger.Info(context.Background(), "served", "status", 200)

	m := decode(t, buf.String())
	if m["module"] != "http" || m["status"] != float64(200) {
		t.Errorf("attached fields missing: %v", m)
	}
}

func TestLogger_With_Chained(t *testing.T) {
	logger, buf := newBufferLogger(logrus.TraceLevel)
	child := logger.With("a", "1").With("b", "2")
	child.Info(context.Background(), "msg")

	m := decode(t, buf.String())
	if m["a"] != "1" || m["b"] != "2" {
		t.Errorf("chained fields missing: %v", m)
	}
}

func TestLogger_With_OddAndNonStringKeys(t *testing.T) {
	logger, buf := newBufferLogger(logrus.TraceLevel)
	child := logger.With(42, "numeric-key-value", "dangling")
	child.Info(context.Background(), "msg")

	m := decode(t, buf.String())
	if m["42"] != "numeric-key-value" {
		t.Errorf("non-string key should be stringified: %v", m)
	}
	if _, ok := m["dangling"]; ok {
		t.Error("dangling key should be dropped")
	}
}

func TestLogger_With_DoesNotAffectParent(t *testing.T) {
	logger, buf := newBufferLogger(logrus.TraceLevel)
	_ = logger.With("module", "http")
	logger.Info(context.Background(), "parent-log")

	m := decode(t, buf.String())
	if _, ok := m["module"]; ok {
		t.Error("With() must not mutate the parent logger")
	}
}

func TestLogger_With_LevelFilteringStillApplies(t *testing.T) {
	logger, buf := newBufferLogger(logrus.WarnLevel)
	child := logger.With("module", "m")
	child.Debug(context.Background(), "hidden")
	if buf.Len() != 0 {
		t.Errorf("derived logger should respect the parent level, got %q", buf.String())
	}
}

// ---------------------------------------------------------------------------
// Constructors and toFields
// ---------------------------------------------------------------------------

func TestNewLogrusLogger(t *testing.T) {
	l := logrus.New()
	logger := NewLogrusLogger(l)
	if logger == nil {
		t.Fatal("NewLogrusLogger returned nil")
	}
	if logger.fields == nil {
		t.Error("fields map should be initialized")
	}

	var _ bLogger.Logger = logger
}

func TestToFields(t *testing.T) {
	tests := []struct {
		name    string
		keyvals []any
		want    logrus.Fields
	}{
		{name: "empty", keyvals: nil, want: logrus.Fields{}},
		{name: "pair", keyvals: []any{"k", "v"}, want: logrus.Fields{"k": "v"}},
		{name: "multiple pairs", keyvals: []any{"a", 1, "b", true}, want: logrus.Fields{"a": 1, "b": true}},
		{name: "odd trailing key dropped", keyvals: []any{"a", 1, "lonely"}, want: logrus.Fields{"a": 1}},
		{name: "non-string key stringified", keyvals: []any{7, "x"}, want: logrus.Fields{"7": "x"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toFields(tt.keyvals)
			if len(got) != len(tt.want) {
				t.Fatalf("toFields(%v) = %v, want %v", tt.keyvals, got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("field %q = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}
