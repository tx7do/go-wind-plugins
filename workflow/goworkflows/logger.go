package goworkflows

import (
	"context"
	"fmt"

	"github.com/tx7do/go-wind/log"
)

const (
	logKey = "[GoWorkflows]"
)

func LogDebug(args ...any) {
	log.GetLogger().Debug(context.Background(), fmt.Sprintf("%s %s", logKey, fmt.Sprint(args...)))
}

func LogInfo(args ...any) {
	log.GetLogger().Info(context.Background(), fmt.Sprintf("%s %s", logKey, fmt.Sprint(args...)))
}

func LogWarn(args ...any) {
	log.GetLogger().Warn(context.Background(), fmt.Sprintf("%s %s", logKey, fmt.Sprint(args...)))
}

func LogError(args ...any) {
	log.GetLogger().Error(context.Background(), fmt.Sprintf("%s %s", logKey, fmt.Sprint(args...)))
}

func LogFatal(args ...any) {
	log.GetLogger().Error(context.Background(), fmt.Sprintf("%s %s", logKey, fmt.Sprint(args...)))
}

func LogDebugf(format string, args ...any) {
	log.GetLogger().Debug(context.Background(), fmt.Sprintf("%s %s", logKey, fmt.Sprintf(format, args...)))
}

func LogInfof(format string, args ...any) {
	log.GetLogger().Info(context.Background(), fmt.Sprintf("%s %s", logKey, fmt.Sprintf(format, args...)))
}

func LogWarnf(format string, args ...any) {
	log.GetLogger().Warn(context.Background(), fmt.Sprintf("%s %s", logKey, fmt.Sprintf(format, args...)))
}

func LogErrorf(format string, args ...any) {
	log.GetLogger().Error(context.Background(), fmt.Sprintf("%s %s", logKey, fmt.Sprintf(format, args...)))
}

func LogFatalf(format string, args ...any) {
	log.GetLogger().Error(context.Background(), fmt.Sprintf("%s %s", logKey, fmt.Sprintf(format, args...)))
}
