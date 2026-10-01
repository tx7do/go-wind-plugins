package langchaingo

import (
	"context"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/prompts"
)

// mockModel is a deterministic llms.Model that records the prompt it receives.
type mockModel struct {
	gotPrompt string
	response  string
}

var _ llms.Model = &mockModel{}

func (m *mockModel) GenerateContent(_ context.Context, messages []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	if m.response == "" {
		m.response = "mock reply"
	}
	// Concatenate all text parts so prompt formatting can be asserted.
	var sb strings.Builder
	for _, mc := range messages {
		for _, part := range mc.Parts {
			if tc, ok := part.(llms.TextContent); ok {
				sb.WriteString(tc.Text)
			}
		}
	}
	m.gotPrompt = sb.String()
	return &llms.ContentResponse{
		Choices: []*llms.ContentChoice{{Content: m.response}},
	}, nil
}

func (m *mockModel) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	resp, err := m.GenerateContent(ctx, []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, prompt),
	}, options...)
	if err != nil {
		return "", err
	}
	return resp.Choices[0].Content, nil
}

// ---------------------------------------------------------------------------
// NewLLMChain
// ---------------------------------------------------------------------------

func TestNewLLMChain_Call(t *testing.T) {
	model := &mockModel{response: "chain reply"}
	prompt := prompts.NewPromptTemplate("Say {{.word}} loudly", []string{"word"})

	chain := NewLLMChain(model, prompt)

	// chains.Call is the documented entry point (LLMChain.Call skips memory
	// handling and raw value maps must contain every template variable).
	out, err := chains.Call(context.Background(), chain, map[string]any{"word": "hello"})
	if err != nil {
		t.Fatalf("chains.Call returned error: %v", err)
	}
	if got, ok := out["text"].(string); !ok || got != "chain reply" {
		t.Errorf("out[text] = %#v, want %q", out["text"], "chain reply")
	}
	// The prompt must have been formatted before reaching the model.
	if model.gotPrompt != "Say hello loudly" {
		t.Errorf("prompt sent to the model = %q, want %q", model.gotPrompt, "Say hello loudly")
	}
}

// ---------------------------------------------------------------------------
// NewConversationChain
// ---------------------------------------------------------------------------

func TestNewConversationChain_DefaultMemory(t *testing.T) {
	model := &mockModel{response: "convo reply"}

	// A nil memory must be replaced by a conversation buffer inside the wrapper.
	chain := NewConversationChain(model, nil)

	out, err := chains.Call(context.Background(), chain, map[string]any{"input": "hi"})
	if err != nil {
		t.Fatalf("chains.Call returned error: %v", err)
	}
	if got, ok := out["text"].(string); !ok || got != "convo reply" {
		t.Errorf("out[text] = %#v, want %q", out["text"], "convo reply")
	}
	// The conversation prompt contains the user input (with an empty history).
	if !strings.Contains(model.gotPrompt, "hi") {
		t.Errorf("prompt sent to the model = %q, want it to contain the input", model.gotPrompt)
	}
}

func TestNewConversationChain_RemembersContext(t *testing.T) {
	ctx := context.Background()
	model := &mockModel{response: "ok"}
	mem := NewConversationBuffer()

	chain := NewConversationChain(model, mem)

	if _, err := chains.Call(ctx, chain, map[string]any{"input": "my name is alice"}); err != nil {
		t.Fatalf("first chains.Call returned error: %v", err)
	}

	// The memory must now contain the first exchange.
	vars, err := mem.LoadMemoryVariables(ctx, nil)
	if err != nil {
		t.Fatalf("LoadMemoryVariables returned error: %v", err)
	}
	history, _ := vars["history"].(string)
	if !strings.Contains(history, "my name is alice") || !strings.Contains(history, "ok") {
		t.Errorf("history = %q, want it to contain the saved exchange", history)
	}
}

// ---------------------------------------------------------------------------
// Document chains (construction only; running them needs a live LLM contract)
// ---------------------------------------------------------------------------

func TestNewStuffDocumentsChain(t *testing.T) {
	model := &mockModel{}
	chain := NewStuffDocumentsChain(NewLLMChain(model, prompts.NewPromptTemplate("Summarize: {{.input}}", []string{"input"})))
	// StuffDocuments is a struct value; its wrapped LLMChain must be wired up.
	if chain.LLMChain == nil {
		t.Fatal("NewStuffDocumentsChain returned a chain without an LLMChain")
	}
}

func TestLoadStuffSummarization(t *testing.T) {
	got := LoadStuffSummarization(&mockModel{})
	if got.LLMChain == nil {
		t.Fatal("LoadStuffSummarization returned a chain without an LLMChain")
	}
}

func TestLoadRefineSummarization(t *testing.T) {
	got := LoadRefineSummarization(&mockModel{})
	if got.LLMChain == nil || got.RefineLLMChain == nil {
		t.Fatal("LoadRefineSummarization returned a chain without LLM chains wired up")
	}
}

func TestLoadMapReduceSummarization(t *testing.T) {
	got := LoadMapReduceSummarization(&mockModel{})
	if got.LLMChain == nil {
		t.Fatal("LoadMapReduceSummarization returned a chain without an LLMChain")
	}
}
