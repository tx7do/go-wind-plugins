// Package saga provides a minimal, fully typed in-process saga orchestrator:
// a sequence of steps with compensations, executed in order and rolled back
// in reverse order when a step fails.
//
//	type state struct{ OrderID, PaymentID string }
//
//	err := saga.Run(ctx, &state{},
//	    saga.Step[state]{
//	        Name: "create-order",
//	        Do:   func(ctx context.Context, s *state) error { s.OrderID = createOrder(); return nil },
//	        Undo: func(ctx context.Context, s *state) error { return deleteOrder(ctx, s.OrderID) },
//	    },
//	    saga.Step[state]{
//	        Name: "charge",
//	        Do:   func(ctx context.Context, s *state) error { return charge(ctx, s.OrderID) },
//	    },
//	)
//
// Run compensates only steps whose Do completed, in reverse order, and
// continues compensating even when an Undo fails (all compensation errors
// are joined with the original error). Undo must be idempotent: a crash
// between Do and Undo, or manual retries, may invoke it more than once.
//
// This orchestrator is in-process: if the whole program dies mid-saga, the
// state is lost. For cross-process durability use a workflow engine such as
// workflow/temporal, or the transaction/outbox pattern when eventual
// consistency is acceptable.
package saga

import (
	"context"
	"errors"
	"fmt"
)

// Step is one saga step over the shared state S. Undo may be nil for steps
// that need no compensation.
type Step[S any] struct {
	// Name identifies the step in errors and logs.
	Name string

	// Do advances the saga. It must not be called again after failure.
	Do func(ctx context.Context, state *S) error

	// Undo compensates a completed Do. It must be idempotent.
	Undo func(ctx context.Context, state *S) error
}

// Run executes the steps in order over state. On the first failure every
// previously completed step is compensated in reverse order. The returned
// error carries the failing step name, the original error, and any
// compensation errors joined by errors.Join.
func Run[S any](ctx context.Context, state *S, steps ...Step[S]) error {
	completed := 0
	for i := range steps {
		step := steps[i]
		if step.Do == nil {
			return fmt.Errorf("saga: step %q has no Do", step.Name)
		}
		if err := step.Do(ctx, state); err != nil {
			return compensate(ctx, state, steps[:completed], step.Name, err)
		}
		completed++
	}
	return nil
}

func compensate[S any](ctx context.Context, state *S, done []Step[S], failedAt string, doErr error) error {
	errs := []error{fmt.Errorf("saga: step %q failed: %w", failedAt, doErr)}
	for i := len(done) - 1; i >= 0; i-- {
		step := done[i]
		if step.Undo == nil {
			continue
		}
		if err := step.Undo(ctx, state); err != nil {
			errs = append(errs, fmt.Errorf("saga: compensate %q: %w", step.Name, err))
		}
	}
	return errors.Join(errs...)
}
