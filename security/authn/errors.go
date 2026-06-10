package authn

import (
	errs "github.com/tx7do/go-wind-plugins/errors"
)

var (
	// ErrorInvalidType is returned when a claim value has an unexpected type.
	ErrorInvalidType = errs.New(errs.StatusInternalServerError, "AUTHN_INVALID_TYPE", "invalid type")

	// --- JWT / token validation errors (401) ---

	ErrInvalidJwtID      = errs.New(errs.StatusUnauthorized, "AUTHN_INVALID_JWT_ID", "invalid jwt id")
	ErrMissingJwtId      = errs.New(errs.StatusUnauthorized, "AUTHN_MISSING_JWT_ID", "jwt id missing")
	ErrInvalidSubject    = errs.New(errs.StatusUnauthorized, "AUTHN_INVALID_SUBJECT", "invalid subject")
	ErrInvalidAudience   = errs.New(errs.StatusUnauthorized, "AUTHN_INVALID_AUDIENCE", "invalid audience")
	ErrInvalidIssuer     = errs.New(errs.StatusUnauthorized, "AUTHN_INVALID_ISSUER", "invalid issuer")
	ErrInvalidExpiration = errs.New(errs.StatusUnauthorized, "AUTHN_INVALID_EXPIRATION", "invalid expiration")
	ErrInvalidNotBefore  = errs.New(errs.StatusUnauthorized, "AUTHN_INVALID_NOT_BEFORE", "invalid not before")
	ErrInvalidIssuedAt   = errs.New(errs.StatusUnauthorized, "AUTHN_INVALID_ISSUED_AT", "invalid issued at")
	ErrInvalidClaims     = errs.New(errs.StatusUnauthorized, "AUTHN_INVALID_CLAIMS", "invalid claims")
	ErrInvalidToken      = errs.New(errs.StatusUnauthorized, "AUTHN_INVALID_TOKEN", "invalid bearer token")

	ErrMissingBearerToken       = errs.New(errs.StatusUnauthorized, "AUTHN_MISSING_BEARER_TOKEN", "missing bearer token")
	ErrUnauthenticated          = errs.New(errs.StatusUnauthorized, "AUTHN_UNAUTHENTICATED", "unauthenticated")
	ErrTokenExpired             = errs.New(errs.StatusUnauthorized, "AUTHN_TOKEN_EXPIRED", "token expired")
	ErrUnsupportedSigningMethod = errs.New(errs.StatusInternalServerError, "AUTHN_UNSUPPORTED_SIGNING_METHOD", "unsupported signing method")
	ErrMissingKeyFunc           = errs.New(errs.StatusInternalServerError, "AUTHN_MISSING_KEY_FUNC", "missing keyFunc")
	ErrSignTokenFailed          = errs.New(errs.StatusInternalServerError, "AUTHN_SIGN_TOKEN_FAILED", "sign token failed")
	ErrGetKeyFailed             = errs.New(errs.StatusInternalServerError, "AUTHN_GET_KEY_FAILED", "get key failed")

	ErrNoAtHash      = errs.New(errs.StatusUnauthorized, "AUTHN_NO_AT_HASH", "id token did not have an access token hash")
	ErrInvalidAtHash = errs.New(errs.StatusUnauthorized, "AUTHN_INVALID_AT_HASH", "access token hash does not match value in ID token")
)
