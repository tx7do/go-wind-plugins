package glog_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/log/glog"
)

// ExampleNewLogger constructs the adapter around glog. glog is flag-driven:
// the host application must call flag.Parse during startup, after which
// verbosity is governed by the -v and -stderrthreshold flags set on the
// command line. Close flushes glog's buffers and belongs in the
// application's shutdown hook.
func ExampleNewLogger() {
	logger := glog.NewLogger()
	defer logger.Close()

	logger.Info(context.Background(), "service started", "port", 8080)
}
