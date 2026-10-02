package authn_test

import (
	"fmt"
	"net/http"

	engine "github.com/tx7do/go-wind-plugins/security/authn"
	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/authn"
)

// ExampleMiddleware attaches the authentication middleware to a server's
// global chain. It authenticates every incoming request with the configured
// authenticator and injects the resulting auth claims into the request
// context for downstream handlers, which retrieve them with
// engine.AuthClaimsFromContext. Unauthenticated requests are answered with a
// 401 response and never reach the handler.
func ExampleMiddleware() {
	srv := windhttp.NewServer(":8080")
	srv.Use(authn.Middleware(nil))

	srv.GET("/api/data", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := engine.AuthClaimsFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		}
		subject, _ := claims.GetSubject()
		fmt.Fprintln(w, "Hello,", subject)
	})

	fmt.Println(srv.Endpoint())
}
