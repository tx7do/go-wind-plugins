package mtls_test

import (
	"context"
	"fmt"

	"github.com/tx7do/go-wind-plugins/security/authn/mtls"
)

// ExampleNewAuthenticator constructs an mTLS authenticator from a static list
// of trusted peer subjects. Unlike Bearer-token schemes, identity here comes
// from the TLS layer: the HTTP middleware or gRPC interceptor extracts the
// subject from the client certificate presented during the handshake and
// attaches it to the request context with ContextWithPeerSubject, after which
// the authenticator checks it against the trusted list. The context helpers
// stand in for that middleware step in this example.
func ExampleNewAuthenticator() {
	auth, err := mtls.NewAuthenticator(
		mtls.WithTrustedCNs([]string{"demo-service"}),
	)
	if err != nil {
		return
	}
	defer auth.Close()

	ctx := mtls.ContextWithPeerSubject(context.Background(), "demo-service")
	_, err = auth.Authenticate(ctx)
	fmt.Println("trusted peer subject accepted:", err == nil)

	ctx = mtls.ContextWithPeerSubject(context.Background(), "unknown-service")
	_, err = auth.Authenticate(ctx)
	fmt.Println("untrusted peer subject accepted:", err == nil)

	// Output:
	// trusted peer subject accepted: true
	// untrusted peer subject accepted: false
}
