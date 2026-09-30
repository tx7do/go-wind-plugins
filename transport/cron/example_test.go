package cron_test

import (
	"fmt"

	crontransport "github.com/tx7do/go-wind-plugins/transport/cron"
)

// ExampleNewServer constructs a cron scheduler and registers timer jobs with
// both a second-level cron expression and the @every descriptor.
func ExampleNewServer() {
	srv := crontransport.NewServer()

	_, _ = srv.NewTimerJob("*/5 * * * * *", func() {})

	_, _ = srv.NewTimerJob("@every 3s", func() {})

	fmt.Println(srv.GetJobCount())
}
