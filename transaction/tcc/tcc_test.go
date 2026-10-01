package tcc_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tx7do/go-wind-plugins/transaction/tcc"
)

type st struct {
	Frozen string
	log    []string
}

func TestRunHappyPath(t *testing.T) {
	var s st

	err := tcc.Run(context.Background(), &s,
		tcc.Participant[st]{
			Name:    "fund",
			Try:     step("try:fund"),
			Confirm: step("confirm:fund"),
			Cancel:  step("cancel:fund"),
		},
		tcc.Participant[st]{
			Name:    "stock",
			Try:     step("try:stock"),
			Confirm: step("confirm:stock"),
			Cancel:  step("cancel:stock"),
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := strings.Join(s.log, ","), "try:fund,try:stock,confirm:fund,confirm:stock"; got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestRunCancelsAllAttemptedInReverseOrder(t *testing.T) {
	var s st
	boom := errors.New("boom")

	err := tcc.Run(context.Background(), &s,
		tcc.Participant[st]{Name: "a", Try: step("try:a"), Confirm: step("confirm:a"), Cancel: step("cancel:a")},
		tcc.Participant[st]{Name: "b", Try: step("try:b"), Confirm: step("confirm:b"), Cancel: step("cancel:b")},
		tcc.Participant[st]{
			Name:    "c",
			Try:     func(context.Context, *st) error { return boom },
			Confirm: step("confirm:c"),
			Cancel:  step("cancel:c"),
		},
	)
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want wrapped boom", err)
	}
	if !strings.Contains(err.Error(), `"c"`) {
		t.Fatalf("error %v does not name the failed participant", err)
	}
	// Cancel includes the failed participant itself (empty cancellation).
	if got, want := strings.Join(s.log, ","), "try:a,try:b,cancel:c,cancel:b,cancel:a"; got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestRunKeepsConfirmingAfterConfirmFailure(t *testing.T) {
	var s st
	confirmErr := errors.New("confirm broken")

	err := tcc.Run(context.Background(), &s,
		tcc.Participant[st]{Name: "a", Try: step("try:a"), Confirm: func(context.Context, *st) error { return confirmErr }, Cancel: step("cancel:a")},
		tcc.Participant[st]{Name: "b", Try: step("try:b"), Confirm: step("confirm:b"), Cancel: step("cancel:b")},
	)
	if !errors.Is(err, confirmErr) {
		t.Fatalf("error = %v, want wrapped confirm error", err)
	}
	// No Cancel after Try succeeded; the remaining Confirm still runs.
	if got, want := strings.Join(s.log, ","), "try:a,try:b,confirm:b"; got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestRunNilCancelSkipped(t *testing.T) {
	var s st

	err := tcc.Run(context.Background(), &s,
		tcc.Participant[st]{Name: "a", Try: step("try:a"), Confirm: step("confirm:a")},
		tcc.Participant[st]{
			Name:    "b",
			Try:     func(context.Context, *st) error { return errors.New("boom") },
			Confirm: step("confirm:b"),
			Cancel:  step("cancel:b"),
		},
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "cancel") {
		t.Fatalf("unexpected cancel error: %v", err)
	}
}

func TestRunValidatesBeforeFirstTry(t *testing.T) {
	var s st

	err := tcc.Run(context.Background(), &s,
		tcc.Participant[st]{Name: "a", Try: step("try:a"), Cancel: step("cancel:a")}, // no Confirm
	)
	if err == nil || !strings.Contains(err.Error(), "no Confirm") {
		t.Fatalf("err = %v, want missing-Confirm error", err)
	}
	if len(s.log) != 0 {
		t.Fatalf("Try ran before validation: %v", s.log)
	}
}

func step(name string) func(context.Context, *st) error {
	return func(_ context.Context, s *st) error {
		s.log = append(s.log, name)
		return nil
	}
}
