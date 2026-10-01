// Package tcc provides a minimal, fully typed in-process TCC (Try-Confirm-
// Cancel) coordinator: each participant reserves resources in Try, commits
// them in Confirm, or releases them in Cancel.
//
//	type state struct{ OrderID, FrozenFundID string }
//
//	err := tcc.Run(ctx, &state{},
//	    tcc.Participant[state]{
//	        Name: "fund",
//	        Try: func(ctx context.Context, s *state) error {
//	            s.FrozenFundID = freezeFunds(ctx, s.OrderID) // reserve
//	            return nil
//	        },
//	        Confirm: func(ctx context.Context, s *state) error {
//	            return settleFunds(ctx, s.FrozenFundID) // commit
//	        },
//	        Cancel: func(ctx context.Context, s *state) error {
//	            return unfreezeFunds(ctx, s.FrozenFundID) // release; idempotent
//	        },
//	    },
//	)
//
// Semantics (matching DTM's TCC):
//
//   - If any Try fails, Cancel runs for every attempted participant —
//     including the failed one (empty cancellation). Cancel must therefore
//     tolerate a Try that never stored a reservation: be idempotent and
//     no-op when there is nothing to release.
//   - Once every Try succeeded, Confirm runs for all participants and is
//     never followed by Cancel. A failing Confirm breaks the TCC invariant
//     (Confirm must eventually succeed); Run continues confirming the
//     remaining participants and joins all Confirm errors for retry/alerting.
//
// This coordinator is in-process: if the whole program dies mid-transaction,
// reservations are stranded and need manual or scheduled cleanup. For
// durable cross-service TCC use a workflow engine such as workflow/temporal,
// or transaction/dtm.
package tcc

import (
	"context"
	"errors"
	"fmt"
)

// Participant is one TCC branch over the shared state S.
type Participant[S any] struct {
	// Name identifies the participant in errors and logs.
	Name string

	// Try reserves resources. On success the reservation identifiers must be
	// recorded into state so Confirm/Cancel can act on them.
	Try func(ctx context.Context, state *S) error

	// Confirm commits the reservation. Required.
	Confirm func(ctx context.Context, state *S) error

	// Cancel releases the reservation. It must be idempotent and tolerate a
	// Try that never completed. Optional only when the branch truly cannot
	// reserve anything.
	Cancel func(ctx context.Context, state *S) error
}

// Run drives Try → Confirm (all succeed) or Try → Cancel (any failure).
func Run[S any](ctx context.Context, state *S, participants ...Participant[S]) error {
	for i := range participants {
		p := participants[i]
		if p.Try == nil {
			return fmt.Errorf("tcc: participant %q has no Try", p.Name)
		}
		if p.Confirm == nil {
			return fmt.Errorf("tcc: participant %q has no Confirm", p.Name)
		}
	}

	for i := range participants {
		p := participants[i]
		if err := p.Try(ctx, state); err != nil {
			return cancel(ctx, state, participants[:i+1], p.Name, err)
		}
	}

	var errs []error
	for _, p := range participants {
		if err := p.Confirm(ctx, state); err != nil {
			errs = append(errs, fmt.Errorf("tcc: confirm %q: %w", p.Name, err))
		}
	}
	return errors.Join(errs...)
}

// cancel releases every attempted participant in reverse order, including
// the one whose Try failed (empty cancellation).
func cancel[S any](ctx context.Context, state *S, attempted []Participant[S], failedAt string, tryErr error) error {
	errs := []error{fmt.Errorf("tcc: try %q failed: %w", failedAt, tryErr)}
	for i := len(attempted) - 1; i >= 0; i-- {
		p := attempted[i]
		if p.Cancel == nil {
			continue
		}
		if err := p.Cancel(ctx, state); err != nil {
			errs = append(errs, fmt.Errorf("tcc: cancel %q: %w", p.Name, err))
		}
	}
	return errors.Join(errs...)
}
