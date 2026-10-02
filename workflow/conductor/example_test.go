package conductor_test

import (
	"github.com/tx7do/go-wind-plugins/workflow/conductor"
)

// ExampleNewClient constructs a Conductor workflow client bound to a server
// URL, with optional key and secret credentials for Orkes Cloud deployments.
// Workflow executions started through the client run against that server,
// and the client must be closed when the application shuts down.
func ExampleNewClient() {
	client, err := conductor.NewClient(
		conductor.ClientOptions{
			ServerURL:  "http://conductor.example.com:8080/api",
			AuthKey:    "auth-key-placeholder",
			AuthSecret: "auth-secret-placeholder",
		},
	)
	if err != nil {
		return
	}
	defer client.Close()
}
