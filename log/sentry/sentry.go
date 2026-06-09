package sentry

import (
	"context"
	"fmt"
	"strconv"

	"github.com/getsentry/sentry-go"

	bLogger "github.com/tx7do/go-wind/log"
)

// Compile-time assertion: sentryLog implements bLogger.Logger.
var _ bLogger.Logger = (*sentryLog)(nil)

// Logger 扩展了项目 Logger 接口，增加 Sentry 特有的方法。
type Logger interface {
	bLogger.Logger

	GetHub() *sentry.Hub
	Close() error
}

// sentryLog 是基于 Sentry SDK 的日志适配器。
//
// Sentry 是业界领先的错误追踪和日志聚合平台。本适配器将日志事件
// 通过 Sentry SDK 发送到 Sentry 平台，Error 和 Warn 级别会作为
// issue 上报，Debug 和 Info 级别作为 breadcrumb 记录。
//
// Example:
//
//	logger, _ := sentry.NewLogger(
//	    sentry.WithDSN("https://xxx@sentry.io/123"),
//	    sentry.WithEnvironment("production"),
//	)
//	defer logger.Close()
type sentryLog struct {
	hub   *sentry.Hub
	extra []any
}

// GetHub 返回底层 Sentry Hub。
func (s *sentryLog) GetHub() *sentry.Hub {
	return s.hub
}

// Close 刷新所有待发送的 Sentry 事件。
func (s *sentryLog) Close() error {
	sentry.Flush(0)
	return nil
}

// Debug 输出 DEBUG 级别日志（记录为 breadcrumb）。
func (s *sentryLog) Debug(ctx context.Context, msg string, keyvals ...any) {
	s.hub.AddBreadcrumb(s.breadcrumb(sentry.LevelDebug, msg, keyvals), nil)
}

// Info 输出 INFO 级别日志（记录为 breadcrumb）。
func (s *sentryLog) Info(ctx context.Context, msg string, keyvals ...any) {
	s.hub.AddBreadcrumb(s.breadcrumb(sentry.LevelInfo, msg, keyvals), nil)
}

// Warn 输出 WARN 级别日志（记录为 breadcrumb）。
func (s *sentryLog) Warn(ctx context.Context, msg string, keyvals ...any) {
	s.hub.AddBreadcrumb(s.breadcrumb(sentry.LevelWarning, msg, keyvals), nil)
}

// Error 输出 ERROR 级别日志（作为事件上报到 Sentry）。
func (s *sentryLog) Error(ctx context.Context, msg string, keyvals ...any) {
	s.hub.CaptureEvent(s.event(sentry.LevelError, msg, keyvals))
}

// With 返回附加了指定 key-value 对的新 Logger 实例。
func (s *sentryLog) With(keyvals ...any) bLogger.Logger {
	return &sentryLog{
		hub:   s.hub,
		extra: append(append([]any{}, s.extra...), keyvals...),
	}
}

// Enabled 报告给定级别是否会被输出。Sentry 默认启用所有级别。
func (s *sentryLog) Enabled(_ bLogger.Level) bool {
	return true
}

// breadcrumb 构建 Sentry Breadcrumb。
func (s *sentryLog) breadcrumb(level sentry.Level, msg string, keyvals []any) *sentry.Breadcrumb {
	bc := &sentry.Breadcrumb{
		Type:    "default",
		Level:   level,
		Message: msg,
		Data:    make(map[string]any),
	}

	all := append(append([]any{}, s.extra...), keyvals...)
	for i := 0; i+1 < len(all); i += 2 {
		bc.Data[fmt.Sprint(all[i])] = all[i+1]
	}

	return bc
}

// event 构建 Sentry Event。
func (s *sentryLog) event(level sentry.Level, msg string, keyvals []any) *sentry.Event {
	evt := sentry.NewEvent()
	evt.Level = level
	evt.Message = msg

	// 将 key-value 对放入 extra 字段
	if evt.Extra == nil {
		evt.Extra = make(map[string]any)
	}

	all := append(append([]any{}, s.extra...), keyvals...)
	for i := 0; i+1 < len(all); i += 2 {
		evt.Extra[fmt.Sprint(all[i])] = all[i+1]
	}

	return evt
}

// NewLogger 创建一个 Sentry 日志记录器。
// 必须提供 DSN 选项。
func NewLogger(opts ...Option) (Logger, error) {
	cfg := defaultOptions()
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.dsn == "" {
		return nil, fmt.Errorf("sentry: DSN is required")
	}

	sentryOpts := sentry.ClientOptions{
		Dsn:              cfg.dsn,
		Environment:      cfg.environment,
		Release:          cfg.release,
		ServerName:       cfg.serverName,
		AttachStacktrace: true,
	}

	if err := sentry.Init(sentryOpts); err != nil {
		return nil, fmt.Errorf("sentry: init: %w", err)
	}

	return &sentryLog{
		hub: sentry.CurrentHub(),
	}, nil
}

// levelToInt 返回级别的可读字符串（调试用途）。
func levelToInt(level bLogger.Level) string {
	return strconv.Itoa(int(level))
}
