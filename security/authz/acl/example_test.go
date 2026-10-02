package acl_test

import (
	"context"
	"fmt"

	"github.com/tx7do/go-wind-plugins/security/authz/acl"
)

// ExampleNewEngine builds an ACL engine seeded with an inline allow rule and
// evaluates two requests against it. The first request matches the rule for
// the demo subject; the second subject has no matching rule and falls through
// to the default deny. In an application the engine would be constructed once
// and consulted by authorization middleware for every guarded request.
func ExampleNewEngine() {
	e, err := acl.NewEngine(
		context.Background(),
		acl.WithRule("demo-user", "read", "demo-resource"),
	)
	if err != nil {
		return
	}

	matched, err := e.IsAuthorized(context.Background(), "demo-user", "read", "demo-resource", "")
	if err != nil {
		return
	}

	unmatched, err := e.IsAuthorized(context.Background(), "demo-other-user", "read", "demo-resource", "")
	if err != nil {
		return
	}

	fmt.Println(matched)
	fmt.Println(unmatched)
	// Output:
	// true
	// false
}
