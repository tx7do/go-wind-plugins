package opa_test

import (
	"context"
	"fmt"

	engine "github.com/tx7do/go-wind-plugins/security/authz"
	"github.com/tx7do/go-wind-plugins/security/authz/opa"
)

// ExampleNewEngine builds an OPA engine on the compiled-in policy modules and
// installs an inline policy document through SetPolicies: one policy makes the
// demo subject a member and carries a single allow statement for one
// demo resource and action. Two requests are then evaluated against that
// store - the pair matching the statement is authorized, the pair targeting a
// different resource is not. In an application the engine would be constructed
// once and consulted by authorization middleware for every guarded request.
func ExampleNewEngine() {
	e, err := opa.NewEngine(context.Background())
	if err != nil {
		return
	}

	err = e.SetPolicies(context.Background(), engine.PolicyMap{
		"demo-policy": map[string]interface{}{
			"members": []string{"demo-user"},
			"statements": map[string]interface{}{
				"demo-statement": map[string]interface{}{
					"effect":    "allow",
					"resources": []string{"demo-resource"},
					"actions":   []string{"demo-action"},
				},
			},
		},
	}, engine.RoleMap{})
	if err != nil {
		return
	}

	matched, err := e.IsAuthorized(context.Background(), "demo-user", "demo-action", "demo-resource", "")
	if err != nil {
		return
	}

	unmatched, err := e.IsAuthorized(context.Background(), "demo-user", "demo-action", "demo-other-resource", "")
	if err != nil {
		return
	}

	fmt.Println(matched)
	fmt.Println(unmatched)
	// Output:
	// true
	// false
}
