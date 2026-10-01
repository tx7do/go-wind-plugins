package langchaingo

import (
	"context"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"
)

// ---------------------------------------------------------------------------
// NewChatMessageHistory
// ---------------------------------------------------------------------------

func TestNewChatMessageHistory_AddAndMessages(t *testing.T) {
	h := NewChatMessageHistory()
	if h == nil {
		t.Fatal("NewChatMessageHistory returned nil")
	}

	if err := h.AddUserMessage(context.Background(), "hello"); err != nil {
		t.Fatalf("AddUserMessage returned error: %v", err)
	}
	if err := h.AddAIMessage(context.Background(), "hi there"); err != nil {
		t.Fatalf("AddAIMessage returned error: %v", err)
	}

	msgs, err := h.Messages(context.Background())
	if err != nil {
		t.Fatalf("Messages returned error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("len(messages) = %d, want 2", len(msgs))
	}
	user, ok := msgs[0].(llms.HumanChatMessage)
	if !ok || user.Content != "hello" {
		t.Errorf("messages[0] = %#v, want HumanChatMessage(hello)", msgs[0])
	}
	ai, ok := msgs[1].(llms.AIChatMessage)
	if !ok || ai.Content != "hi there" {
		t.Errorf("messages[1] = %#v, want AIChatMessage(hi there)", msgs[1])
	}
}

// ---------------------------------------------------------------------------
// NewConversationBuffer
// ---------------------------------------------------------------------------

func TestNewConversationBuffer_Defaults(t *testing.T) {
	mem := NewConversationBuffer()
	if mem == nil {
		t.Fatal("NewConversationBuffer returned nil")
	}
	if key := mem.GetMemoryKey(context.Background()); key != "history" {
		t.Errorf("memory key = %q, want %q", key, "history")
	}

	ctx := context.Background()
	if err := mem.SaveContext(ctx, map[string]any{"input": "hi"}, map[string]any{"output": "hello"}); err != nil {
		t.Fatalf("SaveContext returned error: %v", err)
	}

	vars, err := mem.LoadMemoryVariables(ctx, nil)
	if err != nil {
		t.Fatalf("LoadMemoryVariables returned error: %v", err)
	}
	got, ok := vars["history"].(string)
	if !ok {
		t.Fatalf("history = %#v, want a string", vars["history"])
	}
	want := "Human: hi\nAI: hello"
	if got != want {
		t.Errorf("history = %q, want %q", got, want)
	}
}

func TestNewConversationBuffer_CustomPrefixesAndKey(t *testing.T) {
	ctx := context.Background()
	mem := NewConversationBuffer(
		WithMemoryKey("chat_history"),
		WithHumanPrefix("User"),
		WithAIPrefix("Bot"),
	)
	if key := mem.GetMemoryKey(ctx); key != "chat_history" {
		t.Errorf("memory key = %q, want %q", key, "chat_history")
	}

	if err := mem.SaveContext(ctx, map[string]any{"input": "hi"}, map[string]any{"output": "hello"}); err != nil {
		t.Fatalf("SaveContext returned error: %v", err)
	}

	vars, err := mem.LoadMemoryVariables(ctx, nil)
	if err != nil {
		t.Fatalf("LoadMemoryVariables returned error: %v", err)
	}
	got, ok := vars["chat_history"].(string)
	if !ok {
		t.Fatalf("chat_history = %#v, want a string", vars["chat_history"])
	}
	want := "User: hi\nBot: hello"
	if got != want {
		t.Errorf("chat_history = %q, want %q", got, want)
	}
}

func TestNewConversationBuffer_ReturnMessages(t *testing.T) {
	ctx := context.Background()
	mem := NewConversationBuffer(WithReturnMessages(true))

	if err := mem.SaveContext(ctx, map[string]any{"input": "hi"}, map[string]any{"output": "hello"}); err != nil {
		t.Fatalf("SaveContext returned error: %v", err)
	}

	vars, err := mem.LoadMemoryVariables(ctx, nil)
	if err != nil {
		t.Fatalf("LoadMemoryVariables returned error: %v", err)
	}
	msgs, ok := vars["history"].([]llms.ChatMessage)
	if !ok {
		t.Fatalf("history = %#v, want []llms.ChatMessage", vars["history"])
	}
	if len(msgs) != 2 {
		t.Errorf("len(messages) = %d, want 2", len(msgs))
	}
}

func TestNewConversationBuffer_CustomChatHistory(t *testing.T) {
	ctx := context.Background()
	history := NewChatMessageHistory()
	if err := history.AddUserMessage(ctx, "seeded"); err != nil {
		t.Fatalf("AddUserMessage returned error: %v", err)
	}

	mem := NewConversationBuffer(WithChatHistory(history))
	vars, err := mem.LoadMemoryVariables(ctx, nil)
	if err != nil {
		t.Fatalf("LoadMemoryVariables returned error: %v", err)
	}
	got, ok := vars["history"].(string)
	if !ok || got != "Human: seeded" {
		t.Errorf("history = %q, want %q", got, "Human: seeded")
	}
}

func TestNewConversationBuffer_WithInputOutputKeys(t *testing.T) {
	ctx := context.Background()
	mem := NewConversationBuffer(WithInputKey("question"), WithOutputKey("answer"))

	if err := mem.SaveContext(ctx, map[string]any{"question": "hi"}, map[string]any{"answer": "hello"}); err != nil {
		t.Fatalf("SaveContext returned error: %v", err)
	}

	vars, err := mem.LoadMemoryVariables(ctx, nil)
	if err != nil {
		t.Fatalf("LoadMemoryVariables returned error: %v", err)
	}
	if got, _ := vars["history"].(string); got != "Human: hi\nAI: hello" {
		t.Errorf("history = %q, want %q", got, "Human: hi\nAI: hello")
	}
}

// ---------------------------------------------------------------------------
// NewConversationWindowBuffer
// ---------------------------------------------------------------------------

func TestNewConversationWindowBuffer(t *testing.T) {
	ctx := context.Background()
	mem := NewConversationWindowBuffer(1)

	// Two rounds of conversation, but the window keeps only the most recent one.
	if err := mem.SaveContext(ctx, map[string]any{"input": "first"}, map[string]any{"output": "one"}); err != nil {
		t.Fatalf("SaveContext returned error: %v", err)
	}
	if err := mem.SaveContext(ctx, map[string]any{"input": "second"}, map[string]any{"output": "two"}); err != nil {
		t.Fatalf("SaveContext returned error: %v", err)
	}

	vars, err := mem.LoadMemoryVariables(ctx, nil)
	if err != nil {
		t.Fatalf("LoadMemoryVariables returned error: %v", err)
	}
	got, _ := vars["history"].(string)
	if !strings.Contains(got, "second") || !strings.Contains(got, "two") {
		t.Errorf("history = %q, want it to contain the last round", got)
	}
	if strings.Contains(got, "first") {
		t.Errorf("history = %q, want the first round to be dropped by the window", got)
	}
}

// ---------------------------------------------------------------------------
// NewConversationTokenBuffer (construction only; Load needs a live LLM)
// ---------------------------------------------------------------------------

func TestNewConversationTokenBuffer(t *testing.T) {
	mem := NewConversationTokenBuffer(&mockModel{}, 100)
	if mem == nil {
		t.Fatal("NewConversationTokenBuffer returned nil")
	}
}

// ---------------------------------------------------------------------------
// NewSimpleMemory
// ---------------------------------------------------------------------------

func TestNewSimpleMemory(t *testing.T) {
	ctx := context.Background()
	mem := NewSimpleMemory()

	if vars := mem.MemoryVariables(ctx); len(vars) != 0 {
		t.Errorf("MemoryVariables = %v, want none", vars)
	}

	loaded, err := mem.LoadMemoryVariables(ctx, nil)
	if err != nil {
		t.Fatalf("LoadMemoryVariables returned error: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("LoadMemoryVariables = %v, want empty", loaded)
	}

	if err := mem.SaveContext(ctx, map[string]any{"in": "x"}, map[string]any{"out": "y"}); err != nil {
		t.Errorf("SaveContext returned error: %v", err)
	}
}
