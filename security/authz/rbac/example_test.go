package rbac_test

import (
	"context"
	"fmt"

	"github.com/tx7do/go-wind-plugins/security/authz/rbac"
)

// ExampleNewEngine builds an RBAC engine from an inline role and assignment:
// the demo role carries a single permission for one demo resource and action,
// and the demo subject is enrolled in that role. Two requests are then
// evaluated - the action covered by the role's permission is authorized,
// while the other action is not granted by any role and is denied. In an
// application the engine would be constructed once and consulted by
// authorization middleware for every guarded request.
func ExampleNewEngine() {
	e, err := rbac.NewEngine(
		context.Background(),
		rbac.WithRolePermission("demo-role", "demo-resource", "read"),
		rbac.WithUserRole("demo-user", "demo-role"),
	)
	if err != nil {
		return
	}

	granted, err := e.IsAuthorized(context.Background(), "demo-user", "read", "demo-resource", "")
	if err != nil {
		return
	}

	withheld, err := e.IsAuthorized(context.Background(), "demo-user", "write", "demo-resource", "")
	if err != nil {
		return
	}

	fmt.Println(granted)
	fmt.Println(withheld)
	// Output:
	// true
	// false
}
