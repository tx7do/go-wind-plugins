package presharedkey_test

import (
	"fmt"

	"github.com/tx7do/go-wind-plugins/security/authn/presharedkey"
)

// ExampleNewAuthenticator constructs a preshared-key authenticator from a
// static key list distributed to the authorized clients out of band and
// loaded from configuration at startup. Clients present one of the configured
// keys as a Bearer token; the authn middleware extracts the token from
// request metadata and hands it to the authenticator, which either recognizes
// the key or rejects the request.
func ExampleNewAuthenticator() {
	auth, err := presharedkey.NewAuthenticator(
		presharedkey.WithKeys([]string{"demo-key"}),
	)
	if err != nil {
		return
	}
	defer auth.Close()

	_, err = auth.AuthenticateToken("demo-key")
	fmt.Println("configured key accepted:", err == nil)

	_, err = auth.AuthenticateToken("unknown-key")
	fmt.Println("unknown key accepted:", err == nil)

	// Output:
	// configured key accepted: true
	// unknown key accepted: false
}
