package awsiam_test

import (
	"context"
	"fmt"

	"github.com/tx7do/go-wind-plugins/security/authz/awsiam"
)

// ExampleNewEngine builds an IAM-style engine with a single inline Allow
// statement attached to the demo subject and evaluates two requests against
// it. The first request matches both the action and the resource pattern of
// the statement; the second subject carries no policy at all and falls
// through to the default deny. In an application the engine would be built
// once and queried by authorization middleware for every guarded request.
func ExampleNewEngine() {
	e, err := awsiam.NewEngine(
		context.Background(),
		awsiam.WithAllowStatement(
			"demo-user",
			[]string{"demo-service:GetObject"},
			[]string{"arn:aws:s3:::demo-bucket/*"},
		),
	)
	if err != nil {
		return
	}

	matched, err := e.IsAuthorized(context.Background(), "demo-user", "demo-service:GetObject", "arn:aws:s3:::demo-bucket/object.txt", "")
	if err != nil {
		return
	}

	unmatched, err := e.IsAuthorized(context.Background(), "demo-other-user", "demo-service:GetObject", "arn:aws:s3:::demo-bucket/object.txt", "")
	if err != nil {
		return
	}

	fmt.Println(matched)
	fmt.Println(unmatched)
	// Output:
	// true
	// false
}
