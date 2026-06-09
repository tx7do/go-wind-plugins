package glog

import (
	"context"
	"fmt"
	"strings"

	"github.com/golang/glog"

	bLogger "github.com/tx7do/go-wind/log"
)

// Compile-time assertion: Logger implements bLogger.Logger.
var _ bLogger.Logger = (*Logger)(nil)

// Logger 是基于 Google glog 的日志适配器。
//
// glog 是 Google 开源的 leveled logging 库，广泛用于 gRPC、Kubernetes 等
// 基础设施项目。它按级别分文件输出日志，支持 V() 细粒度日志级别控制。
//
// 注意：glog 不支持原生的结构化键值对，keyvals 会被格式化为 "key=value" 追加到消息中。
//
// Example:
//
//	logger := glog.NewLogger()
//	defer logger.Close()
//	logger.Info(ctx, "server started", "port", 8080)
type Logger struct {
	extra []any // With 持久字段
}

// NewLogger 创建一个 glog 日志记录器。
// 注意：glog 需要在 main() 中调用 flag.Parse() 才能正常工作。
func NewLogger() *Logger {
	return &Logger{}
}

// Debug 输出 DEBUG 级别日志（映射为 glog.V(1)）。
func (l *Logger) Debug(_ context.Context, msg string, keyvals ...any) {
	if glog.V(1) {
		glog.Info(l.format(msg, keyvals))
	}
}

// Info 输出 INFO 级别日志。
func (l *Logger) Info(_ context.Context, msg string, keyvals ...any) {
	glog.Info(l.format(msg, keyvals))
}

// Warn 输出 WARN 级别日志（映射为 glog.Warning）。
func (l *Logger) Warn(_ context.Context, msg string, keyvals ...any) {
	glog.Warning(l.format(msg, keyvals))
}

// Error 输出 ERROR 级别日志。
func (l *Logger) Error(_ context.Context, msg string, keyvals ...any) {
	glog.Error(l.format(msg, keyvals))
}

// With 返回附加了指定 key-value 对的新 Logger 实例。
// glog 不支持原生 With，通过内部 extra 切片模拟。
func (l *Logger) With(keyvals ...any) bLogger.Logger {
	return &Logger{
		extra: append(append([]any{}, l.extra...), keyvals...),
	}
}

// Enabled 报告给定级别是否会被输出。
// glog 的级别由命令行 flag（-v, -stderrthreshold）控制，此处保守返回 true。
func (l *Logger) Enabled(_ bLogger.Level) bool {
	return true
}

// Close 刷新 glog 缓冲区。
func (l *Logger) Close() error {
	glog.Flush()
	return nil
}

// format 将消息和 key-value 对格式化为 glog 兼容的字符串。
// 输出格式："message key1=value1 key2=value2"
func (l *Logger) format(msg string, keyvals []any) string {
	all := append(append([]any{}, l.extra...), keyvals...)
	if len(all) == 0 {
		return msg
	}

	var sb strings.Builder
	sb.WriteString(msg)
	for i := 0; i+1 < len(all); i += 2 {
		sb.WriteString(" ")
		sb.WriteString(fmt.Sprint(all[i]))
		sb.WriteString("=")
		sb.WriteString(fmt.Sprint(all[i+1]))
	}
	return sb.String()
}
