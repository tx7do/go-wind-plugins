package tcc_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/tx7do/go-wind-plugins/transaction/tcc"
)

// demoState stands in for the application state the participants mutate;
// here it only records the order in which the phases run.
type demoState struct {
	log []string
}

// ExampleRun coordinates two TCC participants over a shared state struct.
// Each Participant registers Try (reserve the resource), Confirm (commit the
// reservation), and Cancel (release it; optional only for a branch that can
// reserve nothing). A successful transaction is the sequence printed below —
// every Try first, then every Confirm — and Confirm is never followed by
// Cancel; when a Try fails, Cancel instead runs for every attempted
// participant in reverse order, including the failed one, which may hold no
// reservation, so Cancel must be idempotent. In an application the closures
// wrap the participating services' own resource operations, typically writes
// through their gorm/ent repositories.
func ExampleRun() {
	var s demoState

	err := tcc.Run(context.Background(), &s,
		tcc.Participant[demoState]{
			Name:    "fund",
			Try:     func(_ context.Context, s *demoState) error { s.log = append(s.log, "try:fund"); return nil },
			Confirm: func(_ context.Context, s *demoState) error { s.log = append(s.log, "confirm:fund"); return nil },
			Cancel:  func(_ context.Context, s *demoState) error { s.log = append(s.log, "cancel:fund"); return nil },
		},
		tcc.Participant[demoState]{
			Name:    "stock",
			Try:     func(_ context.Context, s *demoState) error { s.log = append(s.log, "try:stock"); return nil },
			Confirm: func(_ context.Context, s *demoState) error { s.log = append(s.log, "confirm:stock"); return nil },
			Cancel:  func(_ context.Context, s *demoState) error { s.log = append(s.log, "cancel:stock"); return nil },
		},
	)
	if err != nil {
		return
	}

	fmt.Println(strings.Join(s.log, ","))
	// Output: try:fund,try:stock,confirm:fund,confirm:stock
}
