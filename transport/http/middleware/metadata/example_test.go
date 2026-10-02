package metadata_test

import (
	"fmt"
	"net/http"

	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/metadata"
)

// ExampleMiddleware attaches the metadata middleware, which copies the HTTP
// header keys configured with WithKeys from incoming requests into the request
// context. Handlers read those values back through the package-level
// FromContext helper, which yields an empty string for keys that were not
// sent.
//
// Only the configured keys are extracted; every other header is ignored. This
// is the HTTP counterpart of transport/grpc/middleware/metadata. A runnable
// end-to-end setup additionally needs a driver (see transport/http/driver/std).
func ExampleMiddleware() {
	srv := windhttp.NewServer(":8080")
	srv.Use(metadata.Middleware(metadata.WithKeys("X-Tenant-ID")))

	srv.POST("/tenant", func(w http.ResponseWriter, r *http.Request) {
		tenantID := metadata.FromContext(r.Context(), "X-Tenant-ID")
		fmt.Println("tenant:", tenantID)
	})

	fmt.Println(srv.Endpoint())
}
