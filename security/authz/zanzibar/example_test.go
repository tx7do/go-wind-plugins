package zanzibar_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/security/authz/zanzibar"
)

// ExampleNewEngine builds a Zanzibar engine backed by a remote relationship
// store. Every decision is delegated to the configured service, which
// evaluates the relationship tuples recorded there for the given subject,
// action, and resource; the client is chosen at construction time (an OpenFGA
// API as shown here, or a Keto read/write pair). In an application the engine
// would be constructed once and consulted by authorization middleware for
// every guarded request.
func ExampleNewEngine() {
	e, err := zanzibar.NewEngine(
		context.Background(),
		zanzibar.WithOpenFga(
			"http://localhost:8080",
			"demo-store-id",
			nil,
			nil, nil, nil, nil,
		),
	)
	if err != nil {
		return
	}

	ok, err := e.IsAuthorized(context.Background(), "demo-user", "read", "demo-resource", "")
	if err != nil {
		return
	}
	_ = ok
}
