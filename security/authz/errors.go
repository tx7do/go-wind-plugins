package engine

import (
	errs "github.com/tx7do/go-wind-plugins/errors"
)

var (
	ErrMissingAuthClaims = errs.New(errs.StatusForbidden, "AUTHZ_MISSING_CLAIMS", "context missing authz claims")
	ErrInvalidClaims     = errs.New(errs.StatusForbidden, "AUTHZ_INVALID_CLAIMS", "invalid claims")
)
