package machinery_test

import (
	machinerytransport "github.com/tx7do/go-wind-plugins/transport/machinery"
)

func handleDemoTask() error {
	return nil
}

// ExampleNewServer constructs a Machinery worker server and registers a task
// handler for a named task type. Register the server with the application's
// transport lifecycle; the worker then consumes queued task signatures and
// dispatches them to the registered handler.
func ExampleNewServer() {
	srv := machinerytransport.NewServer(
		machinerytransport.WithBrokerAddress("localhost:6379", 0, machinerytransport.BrokerTypeRedis),
		machinerytransport.WithResultBackendAddress("localhost:6379", 0, machinerytransport.BackendTypeRedis),
	)

	if err := srv.HandleFunc("demo_task", handleDemoTask); err != nil {
		return
	}
}
