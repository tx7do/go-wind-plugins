package log

import (
	"context"

	"github.com/tx7do/go-wind/log"
)

// MultiLogger fans out log records to multiple [Logger] instances in order.
// If any logger panics, the remaining loggers still receive the record via
// a deferred recover.
//
// This is useful when you need to send logs to both stderr (for local
// debugging) and a remote collection service simultaneously:
//
//	ml := log.MultiLogger{Loggers: []log.Logger{
//	    log.NewSlogLogger(),
//	    myRemoteLogger,
//	}}
//	log.SetLogger(ml)
type MultiLogger struct {
	// Loggers is the slice of loggers that receive fanned-out records.
	Loggers []log.Logger
}

// fanOut invokes fn for each underlying logger with recover protection so
// that a panicking logger does not abort the remaining fan-out.
func (m MultiLogger) fanOut(fn func(log.Logger)) {
	for _, l := range m.Loggers {
		func() {
			defer func() { recover() }()
			fn(l)
		}()
	}
}

func (m MultiLogger) Debug(ctx context.Context, msg string, args ...any) {
	m.fanOut(func(l log.Logger) { l.Debug(ctx, msg, args...) })
}

func (m MultiLogger) Info(ctx context.Context, msg string, args ...any) {
	m.fanOut(func(l log.Logger) { l.Info(ctx, msg, args...) })
}

func (m MultiLogger) Warn(ctx context.Context, msg string, args ...any) {
	m.fanOut(func(l log.Logger) { l.Warn(ctx, msg, args...) })
}

func (m MultiLogger) Error(ctx context.Context, msg string, args ...any) {
	m.fanOut(func(l log.Logger) { l.Error(ctx, msg, args...) })
}

// Enabled reports whether ANY of the underlying loggers would emit at the
// given level.
func (m MultiLogger) Enabled(level log.Level) bool {
	for _, l := range m.Loggers {
		if l.Enabled(level) {
			return true
		}
	}
	return false
}

// With returns a new MultiLogger whose underlying loggers have the given
// key-value pairs attached.
func (m MultiLogger) With(args ...any) log.Logger {
	children := make([]log.Logger, len(m.Loggers))
	for i, l := range m.Loggers {
		children[i] = l.With(args...)
	}
	return MultiLogger{Loggers: children}
}

// Compile-time assertion: MultiLogger implements Logger.
var _ log.Logger = MultiLogger{}
