package oauth2_test

import (
	"github.com/tx7do/go-wind-plugins/security/authn/oauth2"
)

// ExampleNewAuthenticator constructs an OAuth2 authenticator pointed at the
// token introspection endpoint operated by the authorization server,
// authenticating the introspection request itself with client credentials
// from configuration. At runtime the authn middleware extracts the Bearer
// token from each incoming request and forwards it to
// AuthenticateToken, which consults the endpoint and resolves active tokens
// to the claims reported there. A custom HTTP client with specific timeouts
// or TLS settings can be supplied with WithHTTPClient, and additional claim
// keys to copy with WithExtraClaimsKeys.
func ExampleNewAuthenticator() {
	auth, err := oauth2.NewAuthenticator(
		oauth2.WithIntrospectURL("https://idp.example.com/oauth2/introspect"),
		oauth2.WithClientCredentials("demo-client-id", "demo-client-secret"),
	)
	if err != nil {
		return
	}
	defer auth.Close()

	claims, err := auth.AuthenticateToken("demo-access-token")
	if err != nil {
		return
	}
	_ = claims
}
