package glog

import (
	"context"
	"flag"
	"strings"
	"testing"

	glog "github.com/golang/glog"

	bLogger "github.com/tx7do/go-wind/log"
)

// ---------------------------------------------------------------------------
// format — the pure keyval formatter
// ---------------------------------------------------------------------------

func TestFormat(t *testing.T) {
	tests := map[string]struct {
		logger  *Logger
		msg     string
		keyvals []any
		want    string
	}{
		"message only": {
			logger:  &Logger{},
			msg:     "hello",
			keyvals: nil,
			want:    "hello",
		},
		"empty keyvals slice": {
			logger:  &Logger{},
			msg:     "hello",
			keyvals: []any{},
			want:    "hello",
		},
		"one pair": {
			logger:  &Logger{},
			msg:     "hello",
			keyvals: []any{"k", "v"},
			want:    "hello k=v",
		},
		"multiple pairs": {
			logger:  &Logger{},
			msg:     "hello",
			keyvals: []any{"port", 8080, "env", "prod"},
			want:    "hello port=8080 env=prod",
		},
		"odd trailing keyval is dropped": {
			logger:  &Logger{},
			msg:     "hello",
			keyvals: []any{"k", "v", "lonely"},
			want:    "hello k=v",
		},
		"bool value": {
			logger:  &Logger{},
			msg:     "m",
			keyvals: []any{"ok", true},
			want:    "m ok=true",
		},
		"int value": {
			logger:  &Logger{},
			msg:     "m",
			keyvals: []any{"n", 42},
			want:    "m n=42",
		},
		"nil value": {
			logger:  &Logger{},
			msg:     "m",
			keyvals: []any{"k", nil},
			want:    "m k=<nil>",
		},
		"extra fields from With come first": {
			logger:  &Logger{extra: []any{"mod", "http"}},
			msg:     "hello",
			keyvals: []any{"k", "v"},
			want:    "hello mod=http k=v",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := test.logger.format(test.msg, test.keyvals)
			if got != test.want {
				t.Errorf("format(%q, %v) = %q, want %q", test.msg, test.keyvals, got, test.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// With — persistent keyvals
// ---------------------------------------------------------------------------

func TestWith_ChainsExtras(t *testing.T) {
	root := NewLogger()
	c1 := root.With("a", 1)
	c2 := c1.With("b", 2)

	got := c2.(*Logger).format("m", nil)
	if !strings.Contains(got, "a=1") || !strings.Contains(got, "b=2") {
		t.Errorf("chained With format = %q, want it to contain a=1 and b=2", got)
	}

	// The root logger must stay untouched.
	if got := root.format("m", nil); got != "m" {
		t.Errorf("root logger format = %q, want %q", got, "m")
	}
}

// ---------------------------------------------------------------------------
// Enabled — glog level control is flag based, adapter always reports enabled
// ---------------------------------------------------------------------------

func TestEnabled_AlwaysTrue(t *testing.T) {
	l := NewLogger()
	for _, level := range []bLogger.Level{bLogger.LevelDebug, bLogger.LevelInfo, bLogger.LevelWarn, bLogger.LevelError} {
		if !l.Enabled(level) {
			t.Errorf("Enabled(%v) = false, want true", level)
		}
	}
}

// ---------------------------------------------------------------------------
// Constructors, Close, and level methods (smoke: glog output cannot be
// captured without global flag rewiring, so only exercise the code paths)
// ---------------------------------------------------------------------------

func TestNewLogger(t *testing.T) {
	if l := NewLogger(); l == nil {
		t.Fatal("NewLogger() returned nil")
	}
}

func TestClose_ReturnsNil(t *testing.T) {
	l := NewLogger()
	if err := l.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

func TestLevelMethods_VerboseGating(t *testing.T) {
	t.Cleanup(func() { _ = flag.Set("v", "0") })

	l := NewLogger()
	ctx := context.Background()

	// Default: the "v" flag is 0, so V(1) is disabled and Debug must no-op.
	_ = flag.Set("v", "0")
	if glog.V(1) {
		t.Fatal("glog.V(1) should be disabled while flag v=0")
	}
	l.Debug(ctx, "suppressed") // must not panic and must not log

	// Enable V(1) and exercise all four level methods. glog writes to
	// its own sink (files in the temp dir by default); we only assert
	// that nothing panics and Close (Flush) succeeds.
	_ = flag.Set("v", "1")
	if !glog.V(1) {
		t.Fatal("glog.V(1) should be enabled while flag v=1")
	}
	l.Debug(ctx, "debug message", "k", "v")
	l.Info(ctx, "info message", "k", "v")
	l.Warn(ctx, "warn message")
	l.Error(ctx, "error message")

	if err := l.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}
