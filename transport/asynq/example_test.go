package asynq_test

import (
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for task payloads
	asynqtransport "github.com/tx7do/go-wind-plugins/transport/asynq"
)

type emailPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// ExampleNewServer constructs an Asynq task-queue server backed by Redis.
// Handlers are registered per task type with the generic helpers, which
// deserialize the JSON payload into a typed struct before invoking the
// callback; periodic tasks are registered with a cron expression.
func ExampleNewServer() {
	srv := asynqtransport.NewServer(
		asynqtransport.WithRedisAddress("127.0.0.1:6379"),
		asynqtransport.WithConcurrency(10),
		asynqtransport.WithCodec("json"),
	)

	_ = asynqtransport.RegisterSubscriber[emailPayload](
		srv, "email:send",
		func(taskType string, msg *emailPayload) error {
			_ = taskType
			_ = msg
			return nil
		},
	)

	_, _ = srv.NewPeriodicTask("* * * * *", "email:send", &emailPayload{})
	_ = srv.NewTask("email:send", &emailPayload{})
}
