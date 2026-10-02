package apikey_test

import (
	"fmt"

	"github.com/tx7do/go-wind-plugins/security/authn/apikey"
)

// ExampleNewAuthenticator constructs an API-key authenticator from a static
// key list loaded from configuration at startup. The authn middleware
// extracts the Bearer token from each incoming request and hands it to the
// authenticator, which resolves a configured key to the caller's claims and
// rejects everything else. Keys can also be verified against an external
// source by supplying WithValidator instead of WithKeys.
func ExampleNewAuthenticator() {
	auth, err := apikey.NewAuthenticator(
		apikey.WithKeys([]string{"demo-key"}),
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
