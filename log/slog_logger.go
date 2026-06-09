package log

import (
	"context"
	"log/slog"
	"os"

	"github.com/tx7do/go-wind/log"
)

// SlogLogger adapts the stdlib [*slog.Logger] to the [Logger] interface.
//
// This is the reference implementation returned by [NewSlogLogger]. Callers
// that already own a configured *slog.Logger can wrap it directly:
//
//	log.SetLogger(log.SlogLogger{L: mySlogLogger})
type SlogLogger struct {
	// L is the underlying slog logger. It MUST be non-nil; [NewSlogLogger]
	// always returns a ready-to-use instance.
	L *slog.Logger
}

// Debug forwards to slog.Logger.DebugContext.
func (s SlogLogger) Debug(ctx context.Context, msg string, args ...any) {
	s.L.DebugContext(ensureCtx(ctx), msg, args...)
}

// Info forwards to slog.Logger.InfoContext.
func (s SlogLogger) Info(ctx context.Context, msg string, args ...any) {
	s.L.InfoContext(ensureCtx(ctx), msg, args...)
}

// Warn forwards to slog.Logger.WarnContext.
func (s SlogLogger) Warn(ctx context.Context, msg string, args ...any) {
	s.L.WarnContext(ensureCtx(ctx), msg, args...)
}

// Error forwards to slog.Logger.ErrorContext.
func (s SlogLogger) Error(ctx context.Context, msg string, args ...any) {
	s.L.ErrorContext(ensureCtx(ctx), msg, args...)
}

// Enabled maps the wind [Level] to the equivalent slog level and reports
// whether the underlying [*slog.Logger] would emit a record at that level.
func (s SlogLogger) Enabled(level log.Level) bool {
	return s.L.Enabled(nil, levelToSlog(level))
}

// With returns a new SlogLogger whose underlying *slog.Logger has the given
// key-value pairs attached. This is typically used to distinguish modules,
// e.g., logger.With("module", "registry"). The returned logger will include
// these attributes in every log record it produces.
func (s SlogLogger) With(args ...any) log.Logger {
	return SlogLogger{L: s.L.With(args...)}
}

// ensureCtx returns ctx if non-nil, otherwise context.Background(). This
// prevents nil-context panics in slog handlers.
func ensureCtx(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// levelToSlog maps a wind [Level] to the equivalent [slog.Level].
func levelToSlog(level log.Level) slog.Level {
	switch level {
	case log.LevelDebug:
		return slog.LevelDebug
	case log.LevelInfo:
		return slog.LevelInfo
	case log.LevelWarn:
		return slog.LevelWarn
	case log.LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Compile-time assertion: SlogLogger implements Logger.
var _ log.Logger = SlogLogger{}

// NewSlogLogger builds a [SlogLogger] backed by the stdlib slog with sensible
// defaults: a text handler writing to stderr at INFO level.
//
// Callers needing a different format / level / destination should either:
//   - build their own *slog.Logger and wrap it: SlogLogger{L: myLogger}
//   - or implement the [Logger] interface themselves and pass it to
//     [SetLogger].
func NewSlogLogger() log.Logger {
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	return SlogLogger{L: slog.New(h)}
}
