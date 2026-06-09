package ratelimit

import (
	"context"
	"errors"
	"testing"
)

func TestErrLimited(t *testing.T) {
	if !errors.Is(ErrLimited, ErrLimited) {
		t.Error("ErrLimited should match itself via errors.Is")
	}
	if ErrLimited.Error() == "" {
		t.Error("ErrLimited should have a non-empty message")
	}
}

// dummyLimiter is a minimal Limiter for interface verification.
type dummyLimiter struct{}

func (dummyLimiter) Allow() (bool, error)       { return true, nil }
func (dummyLimiter) Wait(context.Context) error { return nil }
func (dummyLimiter) Close() error               { return nil }

var _ Limiter = dummyLimiter{}

func TestLimiterInterface(t *testing.T) {
	var l Limiter = dummyLimiter{}

	ok, err := l.Allow()
	if err != nil || !ok {
		t.Errorf("Allow() = (%v, %v), want (true, nil)", ok, err)
	}

	if err := l.Wait(context.Background()); err != nil {
		t.Errorf("Wait() = %v, want nil", err)
	}

	if err := l.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}
