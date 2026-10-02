package session_test

import (
	"context"
	"fmt"

	engine "github.com/tx7do/go-wind-plugins/security/authn"
	"github.com/tx7do/go-wind-plugins/security/authn/session"
)

// ExampleNewAuthenticator wires a session authenticator to a session store.
// At login the application stores the authenticated user's claims in the
// store and receives a session ID to hand to the client; on later requests
// the ID arrives with the request, the middleware re-attaches it to the
// request context with ContextWithSessionID, and the authenticator resolves
// it back to the stored claims. NewMemoryStore keeps sessions in process
// memory for development; production deployments supply a distributed
// SessionStore implementation via WithStore.
func ExampleNewAuthenticator() {
	store := session.NewMemoryStore()
	auth, err := session.NewAuthenticator(session.WithStore(store))
	if err != nil {
		return
	}
	defer auth.Close()

	sessionID, err := store.Set("", map[string]interface{}{
		engine.ClaimFieldSubject: "demo-user",
	})
	if err != nil {
		return
	}

	ctx := session.ContextWithSessionID(context.Background(), sessionID)
	_, err = auth.Authenticate(ctx)
	fmt.Println("established session accepted:", err == nil)

	ctx = session.ContextWithSessionID(context.Background(), "unknown-session-id")
	_, err = auth.Authenticate(ctx)
	fmt.Println("unknown session accepted:", err == nil)

	// Output:
	// established session accepted: true
	// unknown session accepted: false
}
