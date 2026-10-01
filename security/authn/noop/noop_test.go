package noop

import (
	"context"
	"testing"

	engine "github.com/tx7do/go-wind-plugins/security/authn"
)

// TestInterfaceCompliance ensures the noop authenticator satisfies the
// authn.Authenticator interface.
func TestInterfaceCompliance(t *testing.T) {
	var _ engine.Authenticator = Authenticator{}
	var _ engine.Authenticator = &Authenticator{}
}

func TestAuthenticate(t *testing.T) {
	n := Authenticator{}
	claims, err := n.Authenticate(context.Background())
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if claims == nil {
		t.Fatal("Authenticate returned nil claims")
	}
	// The noop authenticator returns empty claims.
	if len(*claims) != 0 {
		t.Errorf("Authenticate claims = %v, want empty", *claims)
	}
}

func TestAuthenticateToken(t *testing.T) {
	n := Authenticator{}
	tests := []struct {
		name  string
		token string
	}{
		{"regular token", "bearer-token-123"},
		{"empty token", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := n.AuthenticateToken(tt.token)
			if err != nil {
				t.Fatalf("AuthenticateToken returned error: %v", err)
			}
			if claims == nil {
				t.Fatal("AuthenticateToken returned nil claims")
			}
			if len(*claims) != 0 {
				t.Errorf("AuthenticateToken claims = %v, want empty", *claims)
			}
		})
	}
}

func TestCreateIdentityWithContext(t *testing.T) {
	n := Authenticator{}
	parent := context.WithValue(context.Background(), ctxKeyTest{}, "value") //nolint:staticcheck // test-only key
	claims := engine.AuthClaims{"sub": "alice"}

	got, err := n.CreateIdentityWithContext(parent, claims)
	if err != nil {
		t.Fatalf("CreateIdentityWithContext returned error: %v", err)
	}
	if got != parent {
		t.Error("CreateIdentityWithContext should return the context unchanged")
	}
}

type ctxKeyTest struct{}

func TestCreateIdentity(t *testing.T) {
	n := Authenticator{}
	claims := engine.AuthClaims{"sub": "alice"}

	token, err := n.CreateIdentity(claims)
	if err != nil {
		t.Fatalf("CreateIdentity returned error: %v", err)
	}
	if token != "" {
		t.Errorf("CreateIdentity token = %q, want empty string", token)
	}
}

func TestClose(t *testing.T) {
	// Close must be callable without panicking and be a no-op.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Close panicked: %v", r)
			}
		}()
		n := Authenticator{}
		n.Close()
	}()
}
