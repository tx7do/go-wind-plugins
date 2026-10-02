package basicauth_test

import (
	"encoding/base64"
	"fmt"

	"github.com/tx7do/go-wind-plugins/security/authn/basicauth"
)

// ExampleNewAuthenticator constructs a Basic-Auth authenticator from a static
// credential map loaded from configuration at startup. Clients present
// credentials as a "Basic" Authorization header; the authn middleware
// extracts the encoded token from request metadata and hands it to the
// authenticator, which decodes the username/password pair and either resolves
// it to the caller's claims or rejects it. Production deployments typically
// verify credentials against an external directory by supplying
// WithValidator instead of a static map.
func ExampleNewAuthenticator() {
	auth, err := basicauth.NewAuthenticator(
		basicauth.WithUsers(map[string]string{
			"demo-user": "demo-password",
		}),
	)
	if err != nil {
		return
	}
	defer auth.Close()

	match := base64.StdEncoding.EncodeToString([]byte("demo-user:demo-password"))
	_, err = auth.AuthenticateToken(match)
	fmt.Println("matching credentials accepted:", err == nil)

	mismatch := base64.StdEncoding.EncodeToString([]byte("demo-user:wrong-password"))
	_, err = auth.AuthenticateToken(mismatch)
	fmt.Println("mismatched credentials accepted:", err == nil)

	// Output:
	// matching credentials accepted: true
	// mismatched credentials accepted: false
}
