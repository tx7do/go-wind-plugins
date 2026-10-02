package errors_test

import (
	"fmt"

	"github.com/tx7do/go-wind-plugins/errors"
)

// ExampleNew defines a validation error from a status code, a stable
// machine-readable reason, and a human-readable message. In an application
// such errors are declared once as package-level sentinels and returned by
// business logic; the transport layer later maps the embedded status code to
// its own representation.
func ExampleNew() {
	err := errors.New(errors.StatusBadRequest, "VALIDATION_ERROR", "validation failed")

	fmt.Println(err)
	// Output: VALIDATION_ERROR: validation failed
}

// ExampleFromError recovers a domain error that an intermediate layer has
// wrapped with %w. In an application transport error handlers unwrap errors
// this way to read the status code and reason they carry and translate them
// into the protocol's own representation; an error that wraps no domain error
// yields a nil result so the handler can fall back to its default status.
func ExampleFromError() {
	errNotFound := errors.New(errors.StatusNotFound, "USER_NOT_FOUND", "user not found")
	wrapped := fmt.Errorf("service layer: %w", errNotFound)

	e := errors.FromError(wrapped)
	if e == nil {
		return
	}

	fmt.Println(e.Code, e.Reason)
	// Output: 404 USER_NOT_FOUND
}
