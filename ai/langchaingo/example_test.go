package langchaingo_test

import (
	langchaingo "github.com/tx7do/go-wind-plugins/ai/langchaingo"
)

// ExampleNewModel constructs a cloud LLM client from declarative provider
// settings. The returned model is the shared handle accepted by chains,
// agents, and embedders, so an application wires up higher-level flows by
// passing it to their constructors and can swap providers through
// configuration alone.
func ExampleNewModel() {
	llm, err := langchaingo.NewModel(
		&langchaingo.Config{
			Type:           langchaingo.ModelTypeCloud,
			ModelName:      "gpt-4o",
			TimeoutSeconds: 60,
			Cloud: &langchaingo.CloudConfig{
				ApiKey:  "your-api-key",
				BaseUrl: "https://api.example.com/v1",
			},
		},
	)
	if err != nil {
		return
	}
	_ = llm
}
