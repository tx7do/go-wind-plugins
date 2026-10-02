package validate_test

import (
	"errors"
	"net/http"

	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/validate"
)

// ExampleMiddleware attaches the validation middleware to a server's global
// chain. Before a handler runs, each incoming request is screened against the
// configured rules: an oversized body (whether by declared Content-Length or
// by bytes actually read) is answered with 413 Payload Too Large, a content
// type outside the whitelist with 415 Unsupported Media Type, and a missing
// required header or failing custom validator with 400 Bad Request. A
// runnable end-to-end setup additionally needs a driver (see
// transport/http/driver/std).
func ExampleMiddleware() {
	srv := windhttp.NewServer(":8080")
	srv.Use(validate.Middleware(
		validate.WithMaxBodySize(10<<20), // 10 MiB
		validate.WithAllowedContentTypes("application/json"),
		validate.WithRequiredHeaders("X-Request-ID"),
		validate.WithValidator(func(r *http.Request) error {
			if r.URL.Query().Get("tenant") == "" {
				return errors.New("tenant query parameter is required")
			}
			return nil
		}),
	))
}
