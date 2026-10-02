package cerbos_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/security/authz/cerbos"
)

// ExampleNewEngine builds a Cerbos engine pointed at a Cerbos Policy Decision
// Point. Every decision is delegated to the PDP, which evaluates the YAML
// policies deployed there for the given principal, action, and resource; the
// principal-roles mapping supplies the roles attached to the request, and a
// local evaluator callback can stand in for the remote call during testing.
// In an application the engine would be constructed once and consulted by
// authorization middleware for every guarded request.
func ExampleNewEngine() {
	e, err := cerbos.NewEngine(
		context.Background(),
		cerbos.WithEndpoint("http://localhost:3592"),
		cerbos.WithPrincipalRoles(map[string][]string{
			"demo-user": {"user"},
		}),
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
