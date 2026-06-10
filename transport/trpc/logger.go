package trpc

import (
	"fmt"

	"github.com/tx7do/go-wind/log"
)

const (
	logKey = "[" + KindTRPC + "]"
)

///
/// logger
///

func LogDebug(args ...any) {
	log.GetLogger().Debug(nil, fmt.Sprintf("%s %s", logKey, fmt.Sprint(args...)))
}

func LogInfo(args ...any) {
	log.GetLogger().Info(nil, fmt.Sprintf("%s %s", logKey, fmt.Sprint(args...)))
}

func LogWarn(args ...any) {
	log.GetLogger().Warn(nil, fmt.Sprintf("%s %s", logKey, fmt.Sprint(args...)))
}

func LogError(args ...any) {
	log.GetLogger().Error(nil, fmt.Sprintf("%s %s", logKey, fmt.Sprint(args...)))
}

func LogFatal(args ...any) {
	log.GetLogger().Error(nil, fmt.Sprintf("%s %s", logKey, fmt.Sprint(args...)))
}

///
/// logger
///

func LogDebugf(format string, args ...any) {
	log.GetLogger().Debug(nil, fmt.Sprintf("%s %s", logKey, fmt.Sprintf(format, args...)))
}

func LogInfof(format string, args ...any) {
	log.GetLogger().Info(nil, fmt.Sprintf("%s %s", logKey, fmt.Sprintf(format, args...)))
}

func LogWarnf(format string, args ...any) {
	log.GetLogger().Warn(nil, fmt.Sprintf("%s %s", logKey, fmt.Sprintf(format, args...)))
}

func LogErrorf(format string, args ...any) {
	log.GetLogger().Error(nil, fmt.Sprintf("%s %s", logKey, fmt.Sprintf(format, args...)))
}

func LogFatalf(format string, args ...any) {
	log.GetLogger().Error(nil, fmt.Sprintf("%s %s", logKey, fmt.Sprintf(format, args...)))
}
