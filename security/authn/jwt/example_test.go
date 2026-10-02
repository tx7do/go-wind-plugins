package jwt_test

import (
	"fmt"

	engine "github.com/tx7do/go-wind-plugins/security/authn"
	"github.com/tx7do/go-wind-plugins/security/authn/jwt"
)

// ExampleNewAuthenticator constructs a JWT authenticator for a symmetric
// signing setup: the issuing service signs claims into a token with
// CreateIdentity and sends it as a Bearer token, and the authn middleware on
// the receiving side extracts the token from request metadata and hands it to
// the authenticator, which verifies the signature with the same configured
// key. Asymmetric setups instead pair WithSigningKey with the private key and
// WithVerificationKey with the public key.
func ExampleNewAuthenticator() {
	auth, err := jwt.NewAuthenticator(
		jwt.WithSigningMethod("HS256"),
		jwt.WithKey([]byte("demo-secret")),
	)
	if err != nil {
		return
	}
	defer auth.Close()

	token, err := auth.CreateIdentity(engine.AuthClaims{
		engine.ClaimFieldSubject: "demo-user",
	})
	if err != nil {
		return
	}

	_, err = auth.AuthenticateToken(token)
	fmt.Println("token verified with the issuing key:", err == nil)

	other, err := jwt.NewAuthenticator(
		jwt.WithSigningMethod("HS256"),
		jwt.WithKey([]byte("other-secret")),
	)
	if err != nil {
		return
	}
	defer other.Close()

	_, err = other.AuthenticateToken(token)
	fmt.Println("token verified with a mismatched key:", err == nil)

	// Output:
	// token verified with the issuing key: true
	// token verified with a mismatched key: false
}
