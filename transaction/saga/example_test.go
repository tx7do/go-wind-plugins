package saga_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/tx7do/go-wind-plugins/transaction/saga"
)

// demoState stands in for the application state the steps mutate; here it
// only records the order in which the steps run.
type demoState struct {
	log []string
}

// ExampleRun drives a two-step saga over a shared state struct. Each Step
// registers its forward action and — where the step has effects worth
// undoing — its compensation; a successful saga is simply the ordered
// sequence of forward calls printed below. When a step fails, Run instead
// compensates the completed steps in reverse order and returns the original
// error joined with any compensation errors, which is why Undo must be
// idempotent. In an application the closures wrap the service's own
// operations, typically writes through its gorm/ent repositories; the
// orchestrator is in-process and keeps no durable state.
func ExampleRun() {
	var s demoState

	err := saga.Run(context.Background(), &s,
		saga.Step[demoState]{
			Name: "create-order",
			Do:   func(_ context.Context, s *demoState) error { s.log = append(s.log, "do:create-order"); return nil },
			Undo: func(_ context.Context, s *demoState) error { s.log = append(s.log, "undo:create-order"); return nil },
		},
		saga.Step[demoState]{
			Name: "charge",
			Do:   func(_ context.Context, s *demoState) error { s.log = append(s.log, "do:charge"); return nil },
		},
	)
	if err != nil {
		return
	}

	fmt.Println(strings.Join(s.log, ","))
	// Output: do:create-order,do:charge
}
