package noop

import (
	"context"

	engine "github.com/tx7do/go-wind-plugins/security/authn"
)

type Authenticator struct{}

var _ engine.Authenticator = (*Authenticator)(nil)

func (n Authenticator) Authenticate(_ context.Context) (*engine.AuthClaims, error) {
	return &engine.AuthClaims{}, nil
}

func (n Authenticator) AuthenticateToken(_ string) (*engine.AuthClaims, error) {
	return &engine.AuthClaims{}, nil
}

func (n Authenticator) CreateIdentityWithContext(ctx context.Context, _ engine.AuthClaims) (context.Context, error) {
	return ctx, nil
}

func (n Authenticator) CreateIdentity(_ engine.AuthClaims) (string, error) {
	return "", nil
}

func (n Authenticator) Close() {}
