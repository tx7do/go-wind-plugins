package oidc_test

import (
	"github.com/tx7do/go-wind-plugins/security/authn/oidc"
)

// ExampleNewAuthenticator constructs an OIDC authenticator for a given
// issuer and audience. At startup the constructor fetches the issuer's
// discovery document and signing keys, and at runtime the authn middleware
// extracts the ID token from each incoming request and hands it to
// AuthenticateToken, which verifies the token's signature, issuer, and
// audience against the fetched material before resolving it to claims. The
// issuer URL and expected audience come from configuration.
func ExampleNewAuthenticator() {
	auth, err := oidc.NewAuthenticator(
		oidc.WithIssuerURL("https://idp.example.com"),
		oidc.WithAudience("demo-client"),
	)
	if err != nil {
		return
	}
	defer auth.Close()

	claims, err := auth.AuthenticateToken("demo-id-token")
	if err != nil {
		return
	}
	_ = claims
}
