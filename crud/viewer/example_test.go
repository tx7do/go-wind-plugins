package viewer_test

import (
	"context"
	"fmt"

	"github.com/tx7do/go-wind-plugins/crud/viewer"
)

// ExampleWithContext round-trips an anonymous viewer context through the
// request context. In an application's data access layer middleware injects
// the viewer identity into each request, and the data layer retrieves it to
// classify the view before applying row-level rules; the anonymous context
// carries no identity and is not a platform view.
func ExampleWithContext() {
	ctx := viewer.WithContext(context.Background(), viewer.NewNoopContext())

	vc, ok := viewer.FromContext(ctx)
	fmt.Println(ok, vc.IsPlatformContext())
	// Output: true false
}
