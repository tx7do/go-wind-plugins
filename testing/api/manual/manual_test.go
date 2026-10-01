package api

import (
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
// ChatMessage
// ---------------------------------------------------------------------------

func TestMessageTypeChatConstant(t *testing.T) {
	if MessageTypeChat != 1 {
		t.Errorf("MessageTypeChat = %d, want 1", MessageTypeChat)
	}
}

func TestChatMessage_Construction(t *testing.T) {
	m := ChatMessage{
		Type:    MessageTypeChat,
		Sender:  "alice",
		Message: "hello",
	}

	if m.Type != MessageTypeChat {
		t.Errorf("Type = %d, want %d", m.Type, MessageTypeChat)
	}
	if m.Sender != "alice" {
		t.Errorf("Sender = %q, want %q", m.Sender, "alice")
	}
	if m.Message != "hello" {
		t.Errorf("Message = %q, want %q", m.Message, "hello")
	}
}

func TestChatMessage_JSONRoundTrip(t *testing.T) {
	m := ChatMessage{
		Type:    MessageTypeChat,
		Sender:  "bob",
		Message: "hi there",
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	var got ChatMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if got != m {
		t.Errorf("roundtrip = %+v, want %+v", got, m)
	}
}

func TestChatMessage_JSONFieldNames(t *testing.T) {
	// The struct must serialize under its documented json tags.
	data, err := json.Marshal(ChatMessage{Type: MessageTypeChat, Sender: "s", Message: "m"})
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	for _, key := range []string{"type", "sender", "message"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("JSON output missing key %q: %s", key, data)
		}
	}
}

// ---------------------------------------------------------------------------
// Hygrothermograph
// ---------------------------------------------------------------------------

func TestHygrothermograph_Construction(t *testing.T) {
	h := Hygrothermograph{Humidity: 55.5, Temperature: 23.75}

	if h.Humidity != 55.5 {
		t.Errorf("Humidity = %v, want 55.5", h.Humidity)
	}
	if h.Temperature != 23.75 {
		t.Errorf("Temperature = %v, want 23.75", h.Temperature)
	}
}

func TestHygrothermograph_JSONRoundTrip(t *testing.T) {
	h := Hygrothermograph{Humidity: 40.25, Temperature: 18.5}

	data, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	var got Hygrothermograph
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if got != h {
		t.Errorf("roundtrip = %+v, want %+v", got, h)
	}
}

func TestHygrothermographCreator(t *testing.T) {
	created := HygrothermographCreator()
	if created == nil {
		t.Fatal("HygrothermographCreator returned nil")
	}

	h, ok := created.(*Hygrothermograph)
	if !ok {
		t.Fatalf("HygrothermographCreator returned %T, want *Hygrothermograph", created)
	}
	if h.Humidity != 0 || h.Temperature != 0 {
		t.Errorf("created value = %+v, want zero value", *h)
	}
}
