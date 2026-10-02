package casbin_test

import (
	"context"
	"fmt"

	engine "github.com/tx7do/go-wind-plugins/security/authz"
	"github.com/tx7do/go-wind-plugins/security/authz/casbin"
)

// ExampleNewEngine builds a casbin engine on the default RESTful policy model
// and installs an inline policy through the in-memory adapter: a single rule
// grants the demo subject GET access below /api/demo in every project. Two
// requests are then evaluated - the granted one matches the rule, while the
// other action falls through to deny. In an application the engine would be
// constructed once and consulted by authorization middleware for every
// guarded request.
func ExampleNewEngine() {
	e, err := casbin.NewEngine(context.Background())
	if err != nil {
		return
	}

	err = e.SetPolicies(context.Background(), engine.PolicyMap{
		"policies": []casbin.PolicyRule{
			{PType: "p", V0: "demo-user", V1: "/api/demo/*", V2: "GET", V3: "*"},
		},
	}, nil)
	if err != nil {
		return
	}

	allowed, err := e.IsAuthorized(context.Background(), "demo-user", "GET", "/api/demo/resource", "")
	if err != nil {
		return
	}

	denied, err := e.IsAuthorized(context.Background(), "demo-user", "DELETE", "/api/demo/resource", "")
	if err != nil {
		return
	}

	fmt.Println(allowed)
	fmt.Println(denied)
	// Output:
	// true
	// false
}
