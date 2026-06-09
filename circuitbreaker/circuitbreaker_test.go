package circuitbreaker

import (
	"context"
	"errors"
	"testing"
)

func TestStateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(999), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestErrCircuitOpen(t *testing.T) {
	if !errors.Is(ErrCircuitOpen, ErrCircuitOpen) {
		t.Error("ErrCircuitOpen should match itself via errors.Is")
	}
}

// dummyBreaker is a minimal implementation to verify the interface contract.
type dummyBreaker struct{}

func (dummyBreaker) Allow() error                                     { return nil }
func (dummyBreaker) MarkSuccess()                                     {}
func (dummyBreaker) MarkFailure()                                     {}
func (dummyBreaker) Execute(_ context.Context, fn func() error) error { return fn() }
func (dummyBreaker) State() State                                     { return StateClosed }
func (dummyBreaker) Close() error                                     { return nil }

var _ CircuitBreaker = dummyBreaker{}

func TestCircuitBreakerInterface(t *testing.T) {
	var cb CircuitBreaker = dummyBreaker{}
	if err := cb.Allow(); err != nil {
		t.Errorf("Allow() = %v, want nil", err)
	}
	if s := cb.State(); s != StateClosed {
		t.Errorf("State() = %v, want %v", s, StateClosed)
	}
	if err := cb.Execute(context.Background(), func() error { return nil }); err != nil {
		t.Errorf("Execute() = %v, want nil", err)
	}
	if err := cb.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}
