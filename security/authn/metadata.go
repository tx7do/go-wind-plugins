package authn

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/grpc-ecosystem/go-grpc-middleware/util/metautils"
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
		return "", status.Errorf(codes.Unauthenticated, "Request unauthenticated with %s", expectedScheme)
	}

	splits := strings.SplitN(val, " ", 2)
	if len(splits) < 2 {
		return "", status.Errorf(codes.Unauthenticated, "Bad authorization string")
	}

	if !strings.EqualFold(splits[0], expectedScheme) {
		return "", status.Errorf(codes.Unauthenticated, "Request unauthenticated with %s", expectedScheme)
	}

	return splits[1], nil
}

func formatToken(expectedScheme string, tokenStr string) string {
	return fmt.Sprintf("%s %s", expectedScheme, tokenStr)
}
