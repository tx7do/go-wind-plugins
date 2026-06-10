package authn

import (
	"context"
	"fmt"
	"strings"

	"github.com/grpc-ecosystem/go-grpc-middleware/util/metautils"
	errs "github.com/tx7do/go-wind-plugins/errors"
)

// MDWithAuth injects the token string into the outgoing gRPC metadata of the
// context, formatted as "<expectedScheme> <tokenStr>".
func MDWithAuth(ctx context.Context, expectedScheme string, tokenStr string) context.Context {
	metautils.ExtractOutgoing(ctx).Set(HeaderAuthorize, formatToken(expectedScheme, tokenStr))
	return ctx
}

// AuthFromMD extracts and validates the bearer token from the incoming gRPC
// metadata of the context.
func AuthFromMD(ctx context.Context, expectedScheme string) (string, error) {
	val := metautils.ExtractIncoming(ctx).Get(HeaderAuthorize)
	if val == "" {
		return "", errs.Newf(errs.StatusUnauthorized, "AUTHN_UNAUTHENTICATED", "request unauthenticated with %s", expectedScheme)
	}

	splits := strings.SplitN(val, " ", 2)
	if len(splits) < 2 {
		return "", errs.New(errs.StatusUnauthorized, "AUTHN_BAD_AUTHORIZATION", "bad authorization string")
	}

	if !strings.EqualFold(splits[0], expectedScheme) {
		return "", errs.Newf(errs.StatusUnauthorized, "AUTHN_UNAUTHENTICATED", "request unauthenticated with %s", expectedScheme)
	}

	return splits[1], nil
}

func formatToken(expectedScheme string, tokenStr string) string {
	return fmt.Sprintf("%s %s", expectedScheme, tokenStr)
}
