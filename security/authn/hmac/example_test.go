package hmac_test

import (
	"fmt"

	engine "github.com/tx7do/go-wind-plugins/security/authn"
	"github.com/tx7do/go-wind-plugins/security/authn/hmac"
)

// ExampleNewAuthenticator constructs an HMAC authenticator from a static
// keyID-to-secret map shared between two services. The calling service signs
// each request with CreateIdentity, which produces a token over the keyID and
// a timestamp, and sends it as a Bearer token; the authn middleware on the
// receiving side extracts the token from request metadata and hands it to the
// authenticator, which recomputes the signature with the configured secret
// and enforces the timestamp freshness window. Secrets can also be resolved
// from an external source by supplying WithSecretResolver instead of a static
// map.
func ExampleNewAuthenticator() {
	auth, err := hmac.NewAuthenticator(
		hmac.WithSecrets(map[string]string{
			"demo-key": "demo-secret",
		}),
	)
	if err != nil {
		return
	}
	defer auth.Close()

	token, err := auth.CreateIdentity(engine.AuthClaims{
		engine.ClaimFieldSubject: "demo-key",
	})
	if err != nil {
		return
	}

	_, err = auth.AuthenticateToken(token)
	fmt.Println("request signed with the shared secret accepted:", err == nil)

	other, err := hmac.NewAuthenticator(
		hmac.WithSecrets(map[string]string{
			"demo-key": "other-secret",
		}),
	)
	if err != nil {
		return
	}
	defer other.Close()

	_, err = other.AuthenticateToken(token)
	fmt.Println("request signed with a different secret accepted:", err == nil)

	// Output:
	// request signed with the shared secret accepted: true
	// request signed with a different secret accepted: false
}
