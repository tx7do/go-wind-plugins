package log

import (
	"context"

	"github.com/tx7do/go-wind/log"
)

// LevelFilter wraps a [Logger] and discards messages whose level is below the
// configured [LevelFilter.Level]. This allows callers to control verbosity at
// the call site while keeping the underlying logger unchanged — a natural fit
// for the composable (Lego-like) design philosophy of go-wind.
//
// Usage:
//
//	l := log.LevelFilter{Logger: log.NewSlogLogger(), Level: log.LevelWarn}
//	l.Debug(ctx, "hidden")   // discarded
//	l.Error(ctx, "visible")  // forwarded
type LevelFilter struct {
	// Logger is the underlying logger that receives forwarded messages.
	Logger log.Logger
	// Level is the minimum severity; messages below this level are discarded.
	Level log.Level
}

// Debug forwards to the underlying logger only when the filter threshold
// permits DEBUG level.
func (f LevelFilter) Debug(ctx context.Context, msg string, args ...any) {
	if f.Enabled(log.LevelDebug) {
		f.Logger.Debug(ctx, msg, args...)
	}
}

// Info forwards to the underlying logger only when the filter threshold
// permits INFO level.
func (f LevelFilter) Info(ctx context.Context, msg string, args ...any) {
	if f.Enabled(log.LevelInfo) {
		f.Logger.Info(ctx, msg, args...)
	}
}

// Warn forwards to the underlying logger only when the filter threshold
// permits WARN level.
func (f LevelFilter) Warn(ctx context.Context, msg string, args ...any) {
	if f.Enabled(log.LevelWarn) {
		f.Logger.Warn(ctx, msg, args...)
	}
}

// Error forwards to the underlying logger only when the filter threshold
// permits ERROR level.
func (f LevelFilter) Error(ctx context.Context, msg string, args ...any) {
	if f.Enabled(log.LevelError) {
		f.Logger.Error(ctx, msg, args...)
	}
}

// Enabled reports whether the underlying logger would emit at the given level
// AND that level is at or above the filter threshold.
func (f LevelFilter) Enabled(level log.Level) bool {
	return level >= f.Level && f.Logger.Enabled(level)
}

// With returns a new LevelFilter wrapping the underlying logger's With
// result, preserving the current Level threshold.
func (f LevelFilter) With(args ...any) log.Logger {
	return LevelFilter{
		Logger: f.Logger.With(args...),
		Level:  f.Level,
	}
}

// Compile-time assertion: LevelFilter implements Logger.
var _ log.Logger = LevelFilter{}
