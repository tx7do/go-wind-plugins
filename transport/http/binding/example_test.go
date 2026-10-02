package binding_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/tx7do/go-wind-plugins/transport/http/binding"
)

// ExampleBindQuery shows a query-binding roundtrip that runs entirely in
// memory: a request carrying a fixed query string is built with httptest,
// its query values are bound into a request struct by BindQuery, and the
// populated typed fields are read afterwards.
//
// In an application the request arrives from the framework's HTTP entry
// point rather than a test constructor: the caller extracts the query values
// from the incoming request and passes them to BindQuery together with a
// freshly allocated request struct, so the application handler only ever
// sees the populated typed fields. Request bodies and path parameters are
// bound through the sibling entry points BindBody and BindAllPaths.
func ExampleBindQuery() {
	httpReq := httptest.NewRequest(http.MethodGet, "/items?name=alice&id=42", nil)

	var req struct {
		Name string `json:"name"`
		ID   int    `json:"id"`
	}
	if err := binding.BindQuery(&req, httpReq.URL.Query()); err != nil {
		return
	}

	fmt.Println(req.Name, req.ID)
	// Output: alice 42
}
