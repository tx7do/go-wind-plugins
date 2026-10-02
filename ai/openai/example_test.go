package openai_test

import (
	openai "github.com/tx7do/go-wind-plugins/ai/openai"
)

// ExampleNewClient constructs a client for an OpenAI-compatible cloud
// endpoint. The returned client is the standard handle for the provider's
// API surface, assembled purely from declarative configuration.
func ExampleNewClient() {
	client, err := openai.NewClient(
		&openai.Config{
			Type:           openai.ModelTypeCloud,
			ModelName:      "gpt-4o",
			TimeoutSeconds: 60,
			Cloud: &openai.CloudConfig{
				ApiKey:  "your-api-key",
				BaseUrl: "https://api.example.com/v1",
			},
		},
	)
	if err != nil {
		return
	}
	_ = client
}
