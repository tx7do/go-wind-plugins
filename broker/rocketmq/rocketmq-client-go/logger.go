package rocketmqClientGo

import (
	"fmt"
	"strings"
	"sync"

	"github.com/tx7do/go-wind/log"
)

const (
	loggerKey = "[rocketmq] "
)

var (
	libLoggerMu sync.RWMutex
	libLogger   log.Logger
)

// SetLogger 注入本包内部日志使用的 logger，传入 nil 恢复为框架全局 logger。
// 默认使用框架全局 logger（默认为静默 nop），也可通过 broker.WithLogger 选项在构造时注入。
// 注意：logger 为包级生效，同进程后创建的 broker 会覆盖先注入的。
func SetLogger(l log.Logger) {
	libLoggerMu.Lock()
	defer libLoggerMu.Unlock()
	if l == nil {
		libLogger = nil
		return
	}
	libLogger = l
}

func getLogger() log.Logger {
	libLoggerMu.RLock()
	defer libLoggerMu.RUnlock()
	if libLogger != nil {
		return libLogger
	}
	return log.GetLogger()
}

type logger struct {
	level log.Level
}

func toKeyVals(fields map[string]any) (keyVals []any) {
	for k, v := range fields {
		keyVals = append(keyVals, k)
		keyVals = append(keyVals, v)
	}
	return
}

func (l *logger) logMsg(level log.Level, msg string, fields map[string]any) {
	if l.level > level {
		return
	}
	keyVals := toKeyVals(fields)
	lg := getLogger()
	switch level {
	case log.LevelDebug:
		lg.Debug(nil, loggerKey+msg, keyVals...)
	case log.LevelInfo:
		lg.Info(nil, loggerKey+msg, keyVals...)
	case log.LevelWarn:
		lg.Warn(nil, loggerKey+msg, keyVals...)
	case log.LevelError:
		lg.Error(nil, loggerKey+msg, keyVals...)
	default:
		lg.Error(nil, loggerKey+msg, keyVals...)
	}
}

func (l *logger) logMsgf(level log.Level, format string, a ...any) {
	if l.level > level {
		return
	}
	msg := fmt.Sprintf(format, a...)
	lg := getLogger()
	switch level {
	case log.LevelDebug:
		lg.Debug(nil, loggerKey+msg)
	case log.LevelInfo:
		lg.Info(nil, loggerKey+msg)
	case log.LevelWarn:
		lg.Warn(nil, loggerKey+msg)
	case log.LevelError:
		lg.Error(nil, loggerKey+msg)
	default:
		lg.Error(nil, loggerKey+msg)
	}
}

func (l *logger) Debug(msg string, fields map[string]any) {
	l.logMsg(log.LevelDebug, msg, fields)
}

func (l *logger) Debugf(format string, a ...any) {
	l.logMsgf(log.LevelDebug, format, a...)
}

func (l *logger) Info(msg string, fields map[string]any) {
	l.logMsg(log.LevelInfo, msg, fields)
}

func (l *logger) Infof(format string, a ...any) {
	l.logMsgf(log.LevelInfo, format, a...)
}

func (l *logger) Warning(msg string, fields map[string]any) {
	l.logMsg(log.LevelWarn, msg, fields)
}

func (l *logger) Warningf(format string, a ...any) {
	l.logMsgf(log.LevelWarn, format, a...)
}

func (l *logger) Error(msg string, fields map[string]any) {
	l.logMsg(log.LevelError, msg, fields)
}

func (l *logger) Errorf(format string, a ...any) {
	l.logMsgf(log.LevelError, format, a...)
}

func (l *logger) Fatal(msg string, fields map[string]any) {
	l.logMsg(log.LevelError, msg, fields)
}

func (l *logger) Fatalf(format string, a ...any) {
	l.logMsgf(log.LevelError, format, a...)
}

func (l *logger) Level(lvl string) {
	switch strings.ToLower(lvl) {
	case "panic":
		l.level = log.LevelError
	case "fatal":
		l.level = log.LevelError
	case "error":
		l.level = log.LevelError
	case "warn", "warning":
		l.level = log.LevelWarn
	case "info":
		l.level = log.LevelInfo
	case "debug":
		l.level = log.LevelDebug
	case "trace":
		l.level = log.LevelDebug
	}
}

func (l *logger) OutputPath(_ string) (err error) {
	return nil
}
