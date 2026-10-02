package env_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/config/env"
)

// ExampleNew constructs an environment-variable-backed configuration source.
// The prefix option is prepended to every variable name (turning
// DATABASE_URL into MYAPP_DATABASE_URL), and the key option names the
// variable consulted when Load is called with an empty key.
func ExampleNew() {
	src, err := env.New(
		env.WithPrefix("MYAPP_"),
		env.WithKey("DATABASE_URL"),
	)
	if err != nil {
		return
	}

	raw, err := src.Load(context.Background(), "")
	if err != nil {
		return
	}
	_ = raw
}
