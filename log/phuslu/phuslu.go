package phuslu

import (
	"context"
	"fmt"
	"os"

	"github.com/phuslu/log"

	bLogger "github.com/tx7do/go-wind/log"
)

// Compile-time assertion: Logger implements bLogger.Logger.
var _ bLogger.Logger = (*Logger)(nil)

// Logger 是基于 phuslu/log 的日志适配器。
//
// phuslu/log 是业界性能最高的结构化日志库之一，采用零内存分配设计，
// 在高并发场景下吞吐量显著优于 zap 和 zerolog。
//
// Example:
//
//	// 使用默认配置（控制台 JSON 输出）:
//	logger := phuslu.NewLogger()
//
//	// 使用自定义 phuslu.Logger:
//	pl := &log.Logger{
//	    Level:  log.DebugLevel,
//	    Writer: &log.ConsoleWriter{Writer: os.Stderr},
//	}
//	logger := phuslu.NewLoggerWith(pl)
//	defer logger.Close()
type Logger struct {
	log    *log.Logger
	fields []any // 交替的 key-value 对
}

// NewLogger 创建一个带有默认配置的 phuslu 日志记录器。
// 默认输出到 stderr，JSON 格式，INFO 级别。
func NewLogger() *Logger {
	l := &log.Logger{
		Level:  log.InfoLevel,
		Writer: &log.IOWriter{Writer: os.Stderr},
	}
	return &Logger{log: l}
}

// NewLoggerWith 使用给定的 phuslu.Logger 创建适配器。
func NewLoggerWith(l *log.Logger) *Logger {
	if l == nil {
		return NewLogger()
	}
	return &Logger{log: l}
}

// Debug 输出 DEBUG 级别日志。
func (l *Logger) Debug(_ context.Context, msg string, keyvals ...any) {
	l.logEvent(log.DebugLevel, msg, keyvals)
}

// Info 输出 INFO 级别日志。
func (l *Logger) Info(_ context.Context, msg string, keyvals ...any) {
	l.logEvent(log.InfoLevel, msg, keyvals)
}

// Warn 输出 WARN 级别日志。
func (l *Logger) Warn(_ context.Context, msg string, keyvals ...any) {
	l.logEvent(log.WarnLevel, msg, keyvals)
}

// Error 输出 ERROR 级别日志。
func (l *Logger) Error(_ context.Context, msg string, keyvals ...any) {
	l.logEvent(log.ErrorLevel, msg, keyvals)
}

// With 返回附加了指定 key-value 对的新 Logger 实例。
func (l *Logger) With(keyvals ...any) bLogger.Logger {
	return &Logger{
		log:    l.log,
		fields: append(append([]any{}, l.fields...), keyvals...),
	}
}

// Enabled 报告给定级别是否会被输出。
func (l *Logger) Enabled(level bLogger.Level) bool {
	return levelToPhuslu(level) >= l.log.Level
}

// Close 刷新并关闭底层 phuslu.Logger。
func (l *Logger) Close() error {
	if closer, ok := l.log.Writer.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

// logEvent 通过 builder 模式构造 phuslu 日志条目并写入。
func (l *Logger) logEvent(level log.Level, msg string, keyvals []any) {
	if level < l.log.Level {
		return
	}

	// 按级别获取 Entry builder
	var entry *log.Entry
	switch level {
	case log.DebugLevel:
		entry = l.log.Debug()
	case log.InfoLevel:
		entry = l.log.Info()
	case log.WarnLevel:
		entry = l.log.Warn()
	case log.ErrorLevel:
		entry = l.log.Error()
	default:
		entry = l.log.Info()
	}

	// 附加 With 的持久字段和调用者传入的字段
	all := append(append([]any{}, l.fields...), keyvals...)
	for i := 0; i+1 < len(all); i += 2 {
		entry = entry.Any(fmt.Sprint(all[i]), all[i+1])
	}

	entry.Msg(msg)
}

// levelToPhuslu 将 wind Level 映射为 phuslu Level。
func levelToPhuslu(level bLogger.Level) log.Level {
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
