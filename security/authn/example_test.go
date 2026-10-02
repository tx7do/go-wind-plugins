package authn_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/security/authn"
)

// ExampleContextWithAuthClaims round-trips an identity through the request
// context. Once a credential check has succeeded, the authentication layer
// stores the resulting claims with ContextWithAuthClaims, and handlers further
// down the request path recover them with AuthClaimsFromContext to find out
// who is calling.
func ExampleContextWithAuthClaims() {
	claims := authn.AuthClaims{
		authn.ClaimFieldSubject: "demo-user",
	}

	ctx := authn.ContextWithAuthClaims(context.Background(), &claims)

	stored, ok := authn.AuthClaimsFromContext(ctx)
	if !ok {
		return
	}
	_ = stored
}
