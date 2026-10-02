package engine_test

import (
	"context"

	engine "github.com/tx7do/go-wind-plugins/security/authz"
	_ "github.com/tx7do/go-wind-plugins/security/authz/noop"
)

// ExampleNewEngine builds an authorization engine through the package factory.
// Engine implementations register a factory for their engine.Type at init
// time; the built-in noop engine is linked in here with a blank import, and
// NewEngine dispatches to whatever factory is registered for the requested
// type. Transport middleware would hold the returned Engine and consult it on
// every guarded request; this example performs a single
// subject/action/resource check.
func ExampleNewEngine() {
	eng, err := engine.NewEngine(context.Background(), engine.Noop)
	if err != nil {
		return
	}

	ok, err := eng.IsAuthorized(
		context.Background(),
		engine.Subject("demo-user"),
		engine.Action("read"),
		engine.Resource("demo-resource"),
		engine.Project("demo-project"),
	)
	if err != nil {
		return
	}
	_ = ok
}
