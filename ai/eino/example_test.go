package eino_test

import (
	"context"

	einoOpenai "github.com/cloudwego/eino-ext/components/model/openai"
	eino "github.com/tx7do/go-wind-plugins/ai/eino"
)

// ExampleNewChatModel constructs a chat model backed by an
// OpenAI-compatible cloud endpoint. The resulting model acts as the
// generation node once it is appended to a chain or graph, while the config
// modifier applies provider-native settings such as sampling temperature
// before the underlying client is instantiated.
func ExampleNewChatModel() {
	mdl, err := eino.NewChatModel(
		context.Background(),
		&eino.Config{
			Type:      eino.ModelTypeCloud,
			ModelName: "gpt-4o",
			Cloud: &eino.CloudConfig{
				ApiKey:  "your-api-key",
				BaseUrl: "https://api.example.com/v1",
			},
		},
		eino.WithConfigModifier(func(cfg *einoOpenai.ChatModelConfig) {
			temperature := float32(0.7)
			cfg.Temperature = &temperature
		}),
	)
	if err != nil {
		return
	}
	_ = mdl
}
