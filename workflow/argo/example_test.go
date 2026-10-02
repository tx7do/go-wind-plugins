package argo_test

import (
	"github.com/tx7do/go-wind-plugins/workflow/argo"
)

// ExampleNewClient constructs an Argo Workflows REST client bound to a
// server URL and namespace. All workflow operations issued through the
// client — submission, inspection, and lifecycle control — target that
// server, and the client must be closed when the application shuts down.
func ExampleNewClient() {
	client, err := argo.NewClient(
		argo.ClientOptions{
			ServerURL: "https://argo.example.com:2746",
			Namespace: "default",
		},
	)
	if err != nil {
		return
	}
	defer client.Close()
}
