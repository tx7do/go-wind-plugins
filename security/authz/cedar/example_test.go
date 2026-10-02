package cedar_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/security/authz/cedar"
)

// ExampleNewEngine builds a Cedar engine pointed at an AWS Verified
// Permissions policy store. Every decision is delegated to the remote
// service, which evaluates the Cedar policies held in the store for the
// given principal, action, and resource; a local evaluator callback can be
// injected in place of the remote call for testing. In an application the
// engine would be constructed once and consulted by authorization middleware
// for every guarded request.
func ExampleNewEngine() {
	e, err := cedar.NewEngine(
		context.Background(),
		cedar.WithPolicyStoreID("demo-store-id"),
		cedar.WithRegion("us-east-1"),
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
