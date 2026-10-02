package zerolog

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	bLogger "github.com/tx7do/go-wind/log"
)

// newBufferLogger builds a zerolog-backed Logger writing JSON to buf with no
// timestamps, mirroring the charm/phuslu test setup.
func newBufferLogger(level zerolog.Level) (*Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	zl := zerolog.New(buf).Level(level)
	return NewZerologLogger(&zl), buf
}

// decode parses a single JSON log line into a map for field assertions.
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
		want string // level field value
		msg  string
	}{
		"debug": {call: func(l bLogger.Logger) { l.Debug(context.Background(), "hello", "k", "v") }, want: "debug", msg: "hello"},
		"info":  {call: func(l bLogger.Logger) { l.Info(context.Background(), "hello", "k", "v") }, want: "info", msg: "hello"},
		"warn":  {call: func(l bLogger.Logger) { l.Warn(context.Background(), "hello", "k", "v") }, want: "warn", msg: "hello"},
		"error": {call: func(l bLogger.Logger) { l.Error(context.Background(), "hello", "k", "v") }, want: "error", msg: "hello"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			logger, buf := newBufferLogger(zerolog.TraceLevel)
			tt.call(logger)

			out := buf.String()
			if out == "" {
				t.Fatal("expected log output, got empty buffer")
			}
			m := decode(t, out)
			if m["level"] != tt.want {
				t.Errorf("level = %v, want %v", m["level"], tt.want)
			}
			if m["message"] != tt.msg {
				t.Errorf("msg = %v, want %v", m["message"], tt.msg)
			}
			if m["k"] != "v" {
				t.Errorf("k = %v, want v", m["k"])
			}
		})
	}
}

func TestLogger_ValueTypes(t *testing.T) {
	logger, buf := newBufferLogger(zerolog.TraceLevel)
	logger.Info(context.Background(), "serving",
		"port", 8080,
		"debug", true,
		"ratio", 1.5,
	)

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

func TestLogger_MultipleKeyvals(t *testing.T) {
	logger, buf := newBufferLogger(zerolog.TraceLevel)
	logger.Info(context.Background(), "serving", "port", 8080, "env", "prod")

	m := decode(t, buf.String())
	if m["port"] != float64(8080) || m["env"] != "prod" {
		t.Errorf("keyvals not all present: %v", m)
	}
}

// Odd trailing keyvals are silently dropped by the adapter; the message is
// still emitted.
func TestLogger_OddKeyvals(t *testing.T) {
	logger, buf := newBufferLogger(zerolog.TraceLevel)
	logger.Info(context.Background(), "hello", "lonely-key")

	m := decode(t, buf.String())
	if m["message"] != "hello" {
		t.Errorf("msg = %v, want hello", m["message"])
	}
	if _, ok := m["lonely-key"]; ok {
		t.Error("odd trailing key should be dropped")
	}
}

func TestLogger_NoKeyvals(t *testing.T) {
	logger, buf := newBufferLogger(zerolog.TraceLevel)
	logger.Info(context.Background(), "plain message")

	m := decode(t, buf.String())
	if m["message"] != "plain message" {
		t.Errorf("msg = %v, want %q", m["message"], "plain message")
	}
	if len(m) != 2 { // only level + msg
		t.Errorf("unexpected extra fields: %v", m)
	}
}

// ---------------------------------------------------------------------------
// Level filtering (done by the zerolog core, not the adapter)
// ---------------------------------------------------------------------------

func TestLogger_LevelFiltering(t *testing.T) {
	logger, buf := newBufferLogger(zerolog.WarnLevel)

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
	if m := decode(t, lines[0]); m["message"] != "shown-warn" {
		t.Errorf("first record = %v, want shown-warn", m["message"])
	}
	if m := decode(t, lines[1]); m["message"] != "shown-error" {
		t.Errorf("second record = %v, want shown-error", m["message"])
	}
}

// ---------------------------------------------------------------------------
// Enabled — the adapter reports every level as enabled
// ---------------------------------------------------------------------------

func TestLogger_Enabled(t *testing.T) {
	logger, _ := newBufferLogger(zerolog.ErrorLevel)

	// The wrapper unconditionally returns true; actual filtering happens in
	// the zerolog core. This pins the adapter's (permissive) contract.
	for _, level := range []bLogger.Level{
		bLogger.LevelDebug, bLogger.LevelInfo, bLogger.LevelWarn, bLogger.LevelError,
		bLogger.Level(99),
	} {
		if !logger.Enabled(level) {
			t.Errorf("Enabled(%v) = false, adapter always reports true", level)
		}
	}
}

// ---------------------------------------------------------------------------
// With
// ---------------------------------------------------------------------------

func TestLogger_With(t *testing.T) {
	logger, buf := newBufferLogger(zerolog.TraceLevel)
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
	logger, buf := newBufferLogger(zerolog.TraceLevel)
	child := logger.With("a", "1").With("b", 2)
	child.Info(context.Background(), "msg")

	m := decode(t, buf.String())
	if m["a"] != "1" || m["b"] != float64(2) {
		t.Errorf("chained fields missing: %v", m)
	}
}

func TestLogger_With_OddKeyvalsDropped(t *testing.T) {
	logger, buf := newBufferLogger(zerolog.TraceLevel)
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
	logger, buf := newBufferLogger(zerolog.TraceLevel)
	_ = logger.With("module", "http")
	logger.Info(context.Background(), "parent-log")

	m := decode(t, buf.String())
	if _, ok := m["module"]; ok {
		t.Error("With() must not mutate the parent logger")
	}
}

// ---------------------------------------------------------------------------
// Constructors
// ---------------------------------------------------------------------------

func TestNewZerologLogger(t *testing.T) {
	zl := zerolog.New(&bytes.Buffer{})
	logger := NewZerologLogger(&zl)
	if logger == nil {
		t.Fatal("NewZerologLogger returned nil")
	}
	// The returned value satisfies the framework interface.
	var _ bLogger.Logger = logger
}

// ---------------------------------------------------------------------------
// Writers
// ---------------------------------------------------------------------------

func TestNewStdoutWriter(t *testing.T) {
	if w := NewStdoutWriter(); w == nil {
		t.Error("NewStdoutWriter returned nil")
	}
}

func TestNewConsoleWriter(t *testing.T) {
	if w := NewConsoleWriter(""); w == nil {
		t.Error("NewConsoleWriter(\"\") returned nil")
	}
	if w := NewConsoleWriter("2006-01-02"); w == nil {
		t.Error("NewConsoleWriter(format) returned nil")
	}
}

func TestNewFileWriter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "app.log")

	// closeIO closes the writer if its concrete type supports it; open file
	// handles keep t.TempDir cleanup from succeeding on Windows.
	closeIO := func(w io.Writer) {
		if c, ok := w.(io.Closer); ok {
			_ = c.Close()
		}
	}

	// Parent directories are created and the default mode applied.
	w, err := NewFileWriter(path, 0)
	if err != nil {
		t.Fatalf("NewFileWriter: %v", err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("file content = %q, want %q", data, "hello")
	}

	// Appends on reopen.
	w2, err := NewFileWriter(path, 0o600)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if _, err := w2.Write([]byte("!")); err != nil {
		t.Fatalf("write: %v", err)
	}
	closeIO(w2)
	data, _ = os.ReadFile(path)
	if string(data) != "hello!" {
		t.Errorf("append content = %q, want %q", data, "hello!")
	}
	closeIO(w)

	// Explicit mode is accepted.
	w3, err := NewFileWriter(filepath.Join(dir, "b.log"), 0o600)
	if err != nil {
		t.Errorf("NewFileWriter with explicit mode: %v", err)
	}
	closeIO(w3)

	// Empty path fails.
	if _, err := NewFileWriter("", 0); err == nil {
		t.Error("empty path should fail")
	}
}

func TestNewLumberjackWriter(t *testing.T) {
	dir := t.TempDir()
	w := NewLumberjackWriter(filepath.Join(dir, "app.log"), 1, 2, 3, false)
	if w == nil {
		t.Fatal("NewLumberjackWriter returned nil")
	}
	if _, err := w.Write([]byte("rotating")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if c, ok := w.(io.Closer); ok {
		_ = c.Close()
	}
}

func TestNewMultiWriter(t *testing.T) {
	buf1 := &bytes.Buffer{}
	buf2 := &bytes.Buffer{}
	w := NewMultiWriter(buf1, buf2)
	if _, err := w.Write([]byte("fan-out")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if buf1.String() != "fan-out" || buf2.String() != "fan-out" {
		t.Errorf("both buffers should receive the write: %q / %q", buf1.String(), buf2.String())
	}
}

func TestNewWriter(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name      string
		kind      string
		path      string
		params    map[string]any
		wantErr   bool
		writePath string // when set, do a write through the returned writer
	}{
		{name: "stdout", kind: "stdout"},
		{name: "console with time format", kind: "console", params: map[string]any{"timeFormat": "15:04:05"}},
		{name: "console without params", kind: "console"},
		{name: "file", kind: "file", path: filepath.Join(dir, "a.log"), writePath: "x"},
		{name: "file with mode", kind: "file", path: filepath.Join(dir, "b.log"), params: map[string]any{"mode": os.FileMode(0o600)}, writePath: "x"},
		{
			name: "lumberjack", kind: "lumberjack", path: filepath.Join(dir, "c.log"),
			params: map[string]any{"maxSizeMB": 1, "maxBackups": 2, "maxAge": 3, "compress": false},
		},
		{name: "multi", kind: "multi", params: map[string]any{"writers": []io.Writer{&bytes.Buffer{}, &bytes.Buffer{}}}},
		{name: "file missing path", kind: "file", wantErr: true},
		{name: "lumberjack missing path", kind: "lumberjack", wantErr: true},
		{name: "multi missing params", kind: "multi", wantErr: true},
		{name: "multi wrong writers type", kind: "multi", params: map[string]any{"writers": "nope"}, wantErr: true},
		{name: "unknown kind", kind: "carrier-pigeon", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := NewWriter(tt.kind, tt.path, tt.params)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewWriter(%q) should fail", tt.kind)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewWriter(%q): %v", tt.kind, err)
			}
			if w == nil {
				t.Fatal("NewWriter returned a nil writer without error")
			}
			if tt.writePath != "" {
				if _, err := w.Write([]byte(tt.writePath)); err != nil {
					t.Fatalf("write: %v", err)
				}
			}
			// Release file handles so t.TempDir cleanup works on Windows.
			// Never close stdout/stderr directly, and skip ConsoleWriter,
			// whose Close() delegates to its underlying Out (os.Stdout).
			switch v := w.(type) {
			case *os.File:
				if v != os.Stdout && v != os.Stderr {
					_ = v.Close()
				}
			case zerolog.ConsoleWriter:
				// skip: Close() would close os.Stdout
			case io.Closer:
				_ = v.Close()
			}
		})
	}
}
