package zap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	bLogger "github.com/tx7do/go-wind/log"
)

// bufferSyncer adapts bytes.Buffer to zapcore.WriteSyncer.
type bufferSyncer struct {
	buf bytes.Buffer
}

func (b *bufferSyncer) Write(p []byte) (int, error) { return b.buf.Write(p) }
func (b *bufferSyncer) Sync() error                 { return nil }

// newBufferLogger builds a zap-backed Logger writing JSON to a buffer with
// deterministic formatting (no timestamps, no caller).
func newBufferLogger(level zapcore.Level) (*Logger, *bytes.Buffer) {
	sink := &bufferSyncer{}
	encoderCfg := zapcore.EncoderConfig{
		MessageKey:     "msg",
		LevelKey:       "level",
		NameKey:        "logger",
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	}
	core := zapcore.NewCore(zapcore.NewJSONEncoder(encoderCfg), sink, level)
	return NewZapLogger(zap.New(core)), &sink.buf
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
		"warn":  {call: func(l bLogger.Logger) { l.Warn(context.Background(), "hello", "k", "v") }, want: "warn"},
		"error": {call: func(l bLogger.Logger) { l.Error(context.Background(), "hello", "k", "v") }, want: "error"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			logger, buf := newBufferLogger(zapcore.DebugLevel)
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

func TestLogger_TypedKeyvals(t *testing.T) {
	logger, buf := newBufferLogger(zapcore.DebugLevel)
	logger.Info(context.Background(), "serving", "port", 8080, "debug", true, "ratio", 1.5)

	m := decode(t, buf.String())
	if m["port"] != float64(8080) {
		t.Errorf("port = %v, want 8080", m["port"])
	}
	if m["debug"] != true {
		t.Errorf("debug = %v, want true", m["debug"])
	}
	if m["ratio"] != 1.5 {
		t.Errorf("ratio = %v, want 1.5", m["ratio"])
	}
}

// Odd trailing keyvals are dropped by toFields; the message still emits.
func TestLogger_OddKeyvals(t *testing.T) {
	logger, buf := newBufferLogger(zapcore.DebugLevel)
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
	logger, buf := newBufferLogger(zapcore.DebugLevel)
	logger.Info(context.Background(), "plain message")

	m := decode(t, buf.String())
	if m["msg"] != "plain message" {
		t.Errorf("msg = %v, want %q", m["msg"], "plain message")
	}
	if len(m) != 2 { // level + msg
		t.Errorf("unexpected extra fields: %v", m)
	}
}

// ---------------------------------------------------------------------------
// Level filtering
// ---------------------------------------------------------------------------

func TestLogger_LevelFiltering(t *testing.T) {
	logger, buf := newBufferLogger(zapcore.WarnLevel)

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
		coreLevel zapcore.Level
		level     bLogger.Level
		want      bool
	}{
		{zapcore.InfoLevel, bLogger.LevelDebug, false},
		{zapcore.InfoLevel, bLogger.LevelInfo, true},
		{zapcore.InfoLevel, bLogger.LevelWarn, true},
		{zapcore.InfoLevel, bLogger.LevelError, true},
		{zapcore.DebugLevel, bLogger.LevelDebug, true},
		{zapcore.WarnLevel, bLogger.LevelDebug, false},
		{zapcore.ErrorLevel, bLogger.LevelWarn, false},
		{zapcore.ErrorLevel, bLogger.LevelError, true},
		// Unknown wind levels map to Info.
		{zapcore.InfoLevel, bLogger.Level(99), true},
		{zapcore.DebugLevel, bLogger.Level(99), true},
		{zapcore.ErrorLevel, bLogger.Level(99), false},
	}

	for _, tt := range tests {
		logger, _ := newBufferLogger(tt.coreLevel)
		if got := logger.Enabled(tt.level); got != tt.want {
			t.Errorf("Enabled(%v) with core level %v = %v, want %v",
				tt.level, tt.coreLevel, got, tt.want)
		}
	}
}

func TestLevelToZap(t *testing.T) {
	tests := []struct {
		in   bLogger.Level
		want zapcore.Level
	}{
		{bLogger.LevelDebug, zapcore.DebugLevel},
		{bLogger.LevelInfo, zapcore.InfoLevel},
		{bLogger.LevelWarn, zapcore.WarnLevel},
		{bLogger.LevelError, zapcore.ErrorLevel},
		{bLogger.Level(42), zapcore.InfoLevel},
	}
	for _, tt := range tests {
		if got := levelToZap(tt.in); got != tt.want {
			t.Errorf("levelToZap(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// With
// ---------------------------------------------------------------------------

func TestLogger_With(t *testing.T) {
	logger, buf := newBufferLogger(zapcore.DebugLevel)
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
	logger, buf := newBufferLogger(zapcore.DebugLevel)
	child := logger.With("a", "1").With("b", 2)
	child.Info(context.Background(), "msg")

	m := decode(t, buf.String())
	if m["a"] != "1" || m["b"] != float64(2) {
		t.Errorf("chained fields missing: %v", m)
	}
}

func TestLogger_With_OddKeyvalsDropped(t *testing.T) {
	logger, buf := newBufferLogger(zapcore.DebugLevel)
	child := logger.With("complete", "pair", "dangling")
	child.Info(context.Background(), "msg")

	m := decode(t, buf.String())
	if m["complete"] != "pair" {
		t.Errorf("complete pair missing: %v", m)
	}
	if _, ok := m["dangling"]; ok {
		t.Error("dangling key should be dropped")
	}
}

func TestLogger_With_DoesNotAffectParent(t *testing.T) {
	logger, buf := newBufferLogger(zapcore.DebugLevel)
	_ = logger.With("module", "http")
	logger.Info(context.Background(), "parent-log")

	m := decode(t, buf.String())
	if _, ok := m["module"]; ok {
		t.Error("With() must not mutate the parent logger")
	}
}

// ---------------------------------------------------------------------------
// Sync / Close
// ---------------------------------------------------------------------------

// failingSyncer reports an error from Sync, like a broken file target would.
type failingSyncer struct {
	buf bytes.Buffer
}

func (s *failingSyncer) Write(p []byte) (int, error) { return s.buf.Write(p) }
func (s *failingSyncer) Sync() error                 { return errors.New("sync failed") }

func TestLogger_SyncAndClose(t *testing.T) {
	t.Run("healthy sink syncs cleanly", func(t *testing.T) {
		logger, _ := newBufferLogger(zapcore.DebugLevel)
		if err := logger.Sync(); err != nil {
			t.Errorf("Sync() error = %v, want nil", err)
		}
		if err := logger.Close(); err != nil {
			t.Errorf("Close() error = %v, want nil", err)
		}
	})

	t.Run("sink errors propagate", func(t *testing.T) {
		sink := &failingSyncer{}
		encoderCfg := zapcore.EncoderConfig{
			MessageKey:  "msg",
			LevelKey:    "level",
			EncodeLevel: zapcore.LowercaseLevelEncoder,
		}
		core := zapcore.NewCore(zapcore.NewJSONEncoder(encoderCfg), sink, zapcore.DebugLevel)
		logger := NewZapLogger(zap.New(core))

		if err := logger.Sync(); err == nil || err.Error() != "sync failed" {
			t.Errorf("Sync() error = %v, want sync failed", err)
		}
		if err := logger.Close(); err == nil || err.Error() != "sync failed" {
			t.Errorf("Close() error = %v, want sync failed", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Constructors and toFields
// ---------------------------------------------------------------------------

func TestNewZapLogger(t *testing.T) {
	zl := zap.NewNop()
	logger := NewZapLogger(zl)
	if logger == nil {
		t.Fatal("NewZapLogger returned nil")
	}
	var _ bLogger.Logger = logger
}

func TestToFields(t *testing.T) {
	logger := NewZapLogger(zap.NewNop())

	tests := []struct {
		name    string
		keyvals []any
		wantLen int
	}{
		{name: "empty", keyvals: nil, wantLen: 0},
		{name: "pair", keyvals: []any{"k", "v"}, wantLen: 1},
		{name: "multiple pairs", keyvals: []any{"a", 1, "b", true}, wantLen: 2},
		{name: "odd trailing key dropped", keyvals: []any{"a", 1, "lonely"}, wantLen: 1},
		{name: "non-string key stringified", keyvals: []any{7, "x"}, wantLen: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := logger.toFields(tt.keyvals)
			if len(fields) != tt.wantLen {
				t.Fatalf("toFields(%v) produced %d fields, want %d", tt.keyvals, len(fields), tt.wantLen)
			}
		})
	}

	// Spot-check the field key/value conversion.
	fields := logger.toFields([]any{7, "x"})
	if fields[0].Key != "7" || fields[0].String != "x" {
		t.Errorf("field = %q/%v, want 7/x", fields[0].Key, fields[0].String)
	}
}
