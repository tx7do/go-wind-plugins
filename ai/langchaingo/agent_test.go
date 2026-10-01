package langchaingo

import (
	"testing"

	"github.com/tmc/langchaingo/agents"
)

// All agent constructors are lazy: they only build prompts and chains, so they
// are safe to exercise against the mock model without any network access.

func TestNewOneShotAgent(t *testing.T) {
	agent := NewOneShotAgent(&mockModel{}, nil)
	if agent == nil {
		t.Fatal("NewOneShotAgent returned nil")
	}
}

func TestNewConversationalAgent(t *testing.T) {
	agent := NewConversationalAgent(&mockModel{}, nil)
	if agent == nil {
		t.Fatal("NewConversationalAgent returned nil")
	}
}

func TestNewOpenAIFunctionsAgent(t *testing.T) {
	agent := NewOpenAIFunctionsAgent(&mockModel{}, nil)
	if agent == nil {
		t.Fatal("NewOpenAIFunctionsAgent returned nil")
	}
}

func TestNewExecutor(t *testing.T) {
	agent := NewOneShotAgent(&mockModel{}, nil)
	executor := NewExecutor(agent)
	if executor == nil {
		t.Fatal("NewExecutor returned nil")
	}
}

func TestNewOneShotExecutor(t *testing.T) {
	executor := NewOneShotExecutor(&mockModel{}, nil)
	if executor == nil {
		t.Fatal("NewOneShotExecutor returned nil")
	}
}

func TestNewConversationalExecutor(t *testing.T) {
	executor := NewConversationalExecutor(&mockModel{}, nil)
	if executor == nil {
		t.Fatal("NewConversationalExecutor returned nil")
	}
}

func TestNewOpenAIFunctionsExecutor(t *testing.T) {
	executor := NewOpenAIFunctionsExecutor(&mockModel{}, nil)
	if executor == nil {
		t.Fatal("NewOpenAIFunctionsExecutor returned nil")
	}
}

// Compile-time assertions that the agent types returned by the wrappers still
// satisfy the agents.Agent interface required by NewExecutor.
var (
	_ agents.Agent = (*agents.OneShotZeroAgent)(nil)
	_ agents.Agent = (*agents.ConversationalAgent)(nil)
	_ agents.Agent = (*agents.OpenAIFunctionsAgent)(nil)
)
