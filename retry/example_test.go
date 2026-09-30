package retry_test

import (
	"context"
	"errors"
	"time"

	"github.com/tx7do/go-wind-plugins/retry"
)

func ExampleNew_fixedBackoff() {
	r := retry.New(
		retry.WithMaxAttempts(5),
		retry.WithBackoff(retry.FixedBackoff(200*time.Millisecond)),
		retry.WithMaxTotalWait(2*time.Second),
	)
	_ = r.Do(context.Background(), func(_ context.Context) error {
		return errors.New("transient failure")
	})
}

func ExampleNew_exponentialBackoffWithJitter() {
	r := retry.New(
		retry.WithMaxAttempts(4),
		retry.WithBackoff(retry.ExponentialBackoff{
			Initial: 100 * time.Millisecond,
			Factor:  2,
			Max:     2 * time.Second,
		}),
		retry.WithJitter(retry.FullJitter),
	)
	_ = r.Do(context.Background(), func(_ context.Context) error {
		return nil
	})
}

func ExampleNew_errorClassifier() {
	errTemporary := errors.New("temporary failure")
	errFatal := errors.New("fatal failure")

	r := retry.New(
		retry.WithMaxAttempts(5),
		retry.WithClassifier(retry.RetryIf(func(err error) bool {
			return errors.Is(err, errTemporary)
		})),
	)
	_ = r.Do(context.Background(), func(_ context.Context) error {
		return errFatal
	})
}
