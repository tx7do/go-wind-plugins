package errors_test

import (
	"fmt"
	"net/http"

	errs "github.com/tx7do/go-wind-plugins/errors"
	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	errmw "github.com/tx7do/go-wind-plugins/transport/http/middleware/errors"
)

// errUserNotFound stands in for a sentinel error defined by a service layer,
// as documented in the errors package.
var errUserNotFound = errs.New(http.StatusNotFound, "USER_NOT_FOUND", "user not found")

// fetchUser stands in for a service call that fails.
func fetchUser(string) (string, error) {
	return "", errUserNotFound
}

// ExampleMiddleware attaches the error-encoding middleware, which stores the
// error-response configuration in the request context. Handlers report
// failures through the package-level Respond helper: a framework error is
// translated into its status code and a JSON body carrying the reason and
// message, while any other error becomes a generic 500 response without
// leaking details.
func ExampleMiddleware() {
	srv := windhttp.NewServer(":8080")
	srv.Use(errmw.Middleware())

	srv.GET("/api/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, err := fetchUser(r.PathValue("id"))
		if err != nil {
			errmw.Respond(w, r, err)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	fmt.Println(srv.Endpoint())
}
