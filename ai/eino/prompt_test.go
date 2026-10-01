package eino

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// ---------------------------------------------------------------------------
// Message helpers
// ---------------------------------------------------------------------------

func TestSystemMessage(t *testing.T) {
	m := SystemMessage("you are a helper")
	if m == nil {
		t.Fatal("SystemMessage returned nil")
	}
	if m.Role != schema.System {
		t.Errorf("Role = %q, want %q", m.Role, schema.System)
	}
	if m.Content != "you are a helper" {
		t.Errorf("Content = %q, want %q", m.Content, "you are a helper")
	}
}

func TestUserMessage(t *testing.T) {
	m := UserMessage("hello")
	if m == nil {
		t.Fatal("UserMessage returned nil")
	}
	if m.Role != schema.User {
		t.Errorf("Role = %q, want %q", m.Role, schema.User)
	}
	if m.Content != "hello" {
		t.Errorf("Content = %q, want %q", m.Content, "hello")
	}
}

func TestAssistantMessage(t *testing.T) {
	m := AssistantMessage("hi there")
	if m == nil {
		t.Fatal("AssistantMessage returned nil")
	}
	if m.Role != schema.Assistant {
		t.Errorf("Role = %q, want %q", m.Role, schema.Assistant)
	}
	if m.Content != "hi there" {
		t.Errorf("Content = %q, want %q", m.Content, "hi there")
	}
}

// ---------------------------------------------------------------------------
// FromMessages template formatting
// ---------------------------------------------------------------------------

func TestFromMessages_FString(t *testing.T) {
	tpl := FromMessages(schema.FString,
		SystemMessage("You are a {role} assistant."),
		UserMessage("Question: {question}"),
	)
	if tpl == nil {
		t.Fatal("FromMessages returned nil")
	}

	msgs, err := tpl.Format(context.Background(), map[string]any{
		"role":     "translation",
		"question": "translate this",
	})
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want 2", len(msgs))
	}
	if msgs[0].Role != schema.System {
		t.Errorf("msgs[0].Role = %q, want %q", msgs[0].Role, schema.System)
	}
	if want := "You are a translation assistant."; msgs[0].Content != want {
		t.Errorf("msgs[0].Content = %q, want %q", msgs[0].Content, want)
	}
	if msgs[1].Role != schema.User {
		t.Errorf("msgs[1].Role = %q, want %q", msgs[1].Role, schema.User)
	}
	if want := "Question: translate this"; msgs[1].Content != want {
		t.Errorf("msgs[1].Content = %q, want %q", msgs[1].Content, want)
	}
}

func TestFromMessages_FString_MissingVariable(t *testing.T) {
	tpl := FromMessages(schema.FString, UserMessage("Hello, {name}!"))

	// Formatting with a missing variable should fail, not silently pass through.
	_, err := tpl.Format(context.Background(), map[string]any{})
	if err == nil {
		t.Error("Format with a missing variable should return an error")
	}
}

// ---------------------------------------------------------------------------
// MessagesPlaceholder
// ---------------------------------------------------------------------------

func TestMessagesPlaceholder(t *testing.T) {
	tpl := FromMessages(schema.FString,
		MessagesPlaceholder("history", false),
		UserMessage("{question}"),
	)

	history := []*schema.Message{
		UserMessage("earlier question"),
		AssistantMessage("earlier answer"),
	}
	msgs, err := tpl.Format(context.Background(), map[string]any{
		"history":  history,
		"question": "new question",
	})
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("len(msgs) = %d, want 3", len(msgs))
	}
	if msgs[0].Content != "earlier question" || msgs[0].Role != schema.User {
		t.Errorf("msgs[0] = %s/%q, want user/earlier question", msgs[0].Role, msgs[0].Content)
	}
	if msgs[1].Content != "earlier answer" || msgs[1].Role != schema.Assistant {
		t.Errorf("msgs[1] = %s/%q, want assistant/earlier answer", msgs[1].Role, msgs[1].Content)
	}
	if msgs[2].Content != "new question" {
		t.Errorf("msgs[2].Content = %q, want %q", msgs[2].Content, "new question")
	}
}

func TestMessagesPlaceholder_Optional(t *testing.T) {
	tpl := FromMessages(schema.FString,
		SystemMessage("You are a helper."),
		MessagesPlaceholder("history", true),
		UserMessage("{question}"),
	)

	// An optional placeholder with no value must not fail formatting.
	msgs, err := tpl.Format(context.Background(), map[string]any{"question": "new question"})
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want 2", len(msgs))
	}
	if !strings.Contains(msgs[1].Content, "new question") {
		t.Errorf("msgs[1].Content = %q, want it to contain %q", msgs[1].Content, "new question")
	}
}
