package charm

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/charmbracelet/log"

	bLogger "github.com/tx7do/go-wind/log"
)

// Compile-time assertion: Logger implements bLogger.Logger.
var _ bLogger.Logger = (*Logger)(nil)

// Logger 是基于 Charmbracelet log 的日志适配器。
//
// charm/log 提供色彩丰富、人类友好的终端日志输出，支持日志级别、
// 结构化键值对、调用栈报告等功能。非常适合本地开发和调试场景。
//
// Example:
//
//	logger := charm.NewLogger()
//	// 带颜色输出的控制台日志：
//	logger.Info(ctx, "server started", "port", 8080)
type Logger struct {
	log *log.Logger
}

// NewLogger 创建一个带有默认配置的 charm 日志记录器。
// 默认输出到 stderr，INFO 级别，彩色输出。
func NewLogger() *Logger {
	l := log.New(os.Stderr)
	l.SetLevel(log.InfoLevel)
	l.SetReportCaller(false)
	l.SetReportTimestamp(true)
	return &Logger{log: l}
}

// NewLoggerWith 使用给定的 charm log.Logger 创建适配器。
func NewLoggerWith(l *log.Logger) *Logger {
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
// 使用 charm log 原生的 With() 方法。
func (l *Logger) With(keyvals ...any) bLogger.Logger {
	return &Logger{log: l.log.With(keyvals...)}
}

// Enabled 报告给定级别是否会被输出。
func (l *Logger) Enabled(level bLogger.Level) bool {
	return levelToCharm(level) >= l.log.GetLevel()
}

// Close 是一个空操作。charm log 写入到 io.Writer，无需显式关闭。
func (l *Logger) Close() error {
	return nil
}

// levelToCharm 将 wind Level 映射为 charm log.Level。
func levelToCharm(level bLogger.Level) log.Level {
	switch level {
	case bLogger.LevelDebug:
		return log.DebugLevel
	case bLogger.LevelInfo:
		return log.InfoLevel
	case bLogger.LevelWarn:
		return log.WarnLevel
	case bLogger.LevelError:
		return log.ErrorLevel
	default:
		return log.InfoLevel
	}
}

// String 返回当前 Logger 的可读描述。
func (l *Logger) String() string {
	return "charm.Logger{level=" + strconv.Itoa(int(l.log.GetLevel())) + ", prefix=" + fmt.Sprint(l.log.GetPrefix()) + "}"
}
