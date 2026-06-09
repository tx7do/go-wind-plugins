package hclog

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/go-hclog"

	bLogger "github.com/tx7do/go-wind/log"
)

// Compile-time assertion: Logger implements bLogger.Logger.
var _ bLogger.Logger = (*Logger)(nil)

// Logger 是基于 HashiCorp hclog 的日志适配器。
//
// hclog 是 HashiCorp 全家桶（Consul / Vault / Nomad / Terraform）的御用日志库，
// 支持结构化日志、级别继承、Named Logger（子日志器自动附加前缀）等特性。
// 适配 hclog 后可与 HashiCorp 生态组件互操作。
//
// Example:
//
//	// 使用默认配置:
//	logger := hclog.NewLogger()
//
//	// 使用自定义 hclog.Logger:
//	hl := hclog.New(&hclog.LoggerOptions{
//	    Name:  "myapp",
//	    Level: hclog.Debug,
//	})
//	logger := hclog.NewLoggerWith(hl)
type Logger struct {
	log hclog.Logger
}

// NewLogger 创建一个带有默认配置的 hclog 日志记录器。
// 默认输出到 stderr，JSON 格式，INFO 级别。
func NewLogger() *Logger {
	l := hclog.New(&hclog.LoggerOptions{
		Name:       "app",
		Level:      hclog.Info,
		Output:     os.Stderr,
		JSONFormat: true,
	})
	return &Logger{log: l}
}

// NewLoggerWith 使用给定的 hclog.Logger 创建适配器。
func NewLoggerWith(l hclog.Logger) *Logger {
	if l == nil {
		return NewLogger()
	}
	return &Logger{log: l}
}

// Debug 输出 DEBUG 级别日志。
func (l *Logger) Debug(_ context.Context, msg string, keyvals ...any) {
	l.log.Debug(msg, keyvals...)
}

// Info 输出 INFO 级别日志。
func (l *Logger) Info(_ context.Context, msg string, keyvals ...any) {
	l.log.Info(msg, keyvals...)
}

// Warn 输出 WARN 级别日志。
func (l *Logger) Warn(_ context.Context, msg string, keyvals ...any) {
	l.log.Warn(msg, keyvals...)
}

// Error 输出 ERROR 级别日志。
func (l *Logger) Error(_ context.Context, msg string, keyvals ...any) {
	l.log.Error(msg, keyvals...)
}

// With 返回附加了指定 key-value 对的新 Logger 实例。
// 使用 hclog 原生的 With() 机制。
func (l *Logger) With(keyvals ...any) bLogger.Logger {
	return &Logger{log: l.log.With(keyvals...)}
}

// Enabled 报告给定级别是否会被输出。
func (l *Logger) Enabled(level bLogger.Level) bool {
	return levelToHclog(level) >= l.log.GetLevel()
}

// Close 是一个空操作。hclog.Logger 接口本身不提供 Flush/Close 方法。
// 如果底层 Writer 支持 Close（如文件），用户应自行管理。
func (l *Logger) Close() error {
	return nil
}

// Name 返回当前 Logger 的名称（hclog 特有）。
func (l *Logger) Name() string {
	return l.log.Name()
}

// Named 返回一个带有子名称的新 Logger（hclog 特有）。
func (l *Logger) Named(name string) bLogger.Logger {
	return &Logger{log: l.log.Named(name)}
}

// levelToHclog 将 wind Level 映射为 hclog.Level。
func levelToHclog(level bLogger.Level) hclog.Level {
	switch level {
	case bLogger.LevelDebug:
		return hclog.Debug
	case bLogger.LevelInfo:
		return hclog.Info
	case bLogger.LevelWarn:
		return hclog.Warn
	case bLogger.LevelError:
		return hclog.Error
	default:
		return hclog.Info
	}
}

// String 返回当前 Logger 的可读描述。
func (l *Logger) String() string {
	return fmt.Sprintf("hclog.Logger{name=%s}", l.log.Name())
}
