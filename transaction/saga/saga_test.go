package saga_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tx7do/go-wind-plugins/transaction/saga"
)

type st struct {
	N   int
	log []string
}

func TestRunHappyPath(t *testing.T) {
	var s st

	err := saga.Run(context.Background(), &s,
		saga.Step[st]{
			Name: "one",
			Do:   func(_ context.Context, s *st) error { s.N += 1; s.log = append(s.log, "do:one"); return nil },
			Undo: func(_ context.Context, s *st) error { s.N -= 1; s.log = append(s.log, "undo:one"); return nil },
		},
		saga.Step[st]{
			Name: "two",
			Do:   func(_ context.Context, s *st) error { s.N *= 2; s.log = append(s.log, "do:two"); return nil },
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.N != 2 {
		t.Fatalf("state N = %d, want 2", s.N)
	}
	if got, want := strings.Join(s.log, ","), "do:one,do:two"; got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestRunCompensatesInReverseOrder(t *testing.T) {
	var s st
	boom := errors.New("boom")

	err := saga.Run(context.Background(), &s,
		saga.Step[st]{Name: "a", Do: step("do:a"), Undo: step("undo:a")},
		saga.Step[st]{Name: "b", Do: step("do:b"), Undo: step("undo:b")},
		saga.Step[st]{Name: "c", Do: func(context.Context, *st) error { return boom }},
	)
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want wrapped boom", err)
	}
	if !strings.Contains(err.Error(), `"c"`) {
		t.Fatalf("error %v does not name the failing step", err)
	}
	if got, want := strings.Join(s.log, ","), "do:a,do:b,undo:b,undo:a"; got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestRunKeepsCompensatingAfterUndoFailure(t *testing.T) {
	var s st
	undoErr := errors.New("undo broken")

	err := saga.Run(context.Background(), &s,
		saga.Step[st]{Name: "a", Do: step("do:a"), Undo: func(context.Context, *st) error { return undoErr }},
		saga.Step[st]{Name: "b", Do: step("do:b"), Undo: step("undo:b")},
		saga.Step[st]{Name: "c", Do: func(context.Context, *st) error { return errors.New("boom") }},
	)
	if !errors.Is(err, undoErr) {
		t.Fatalf("error = %v, want joined undo error", err)
	}
	if got, want := strings.Join(s.log, ","), "do:a,do:b,undo:b"; got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestRunNilUndoSkipped(t *testing.T) {
	var s st

	err := saga.Run(context.Background(), &s,
		saga.Step[st]{Name: "a", Do: step("do:a")},
		saga.Step[st]{Name: "b", Do: func(context.Context, *st) error { return errors.New("boom") }},
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "compensate") {
		t.Fatalf("unexpected compensation error: %v", err)
	}
}

func TestRunNilDoRejected(t *testing.T) {
	err := saga.Run(context.Background(), &st{}, saga.Step[st]{Name: "empty"})
	if err == nil || !strings.Contains(err.Error(), "no Do") {
		t.Fatalf("err = %v, want missing-Do error", err)
	}
}

func step(name string) func(context.Context, *st) error {
	return func(_ context.Context, s *st) error {
		s.log = append(s.log, name)
		return nil
	}
}
