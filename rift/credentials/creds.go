package credentials

import (
	"context"
	"regexp"

	"github.com/rotisserie/eris"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var (
	TokenKey = "router_key"

	_ credentials.PerRPCCredentials = &tokenCredential{}

	routerKeyRegexp = regexp.MustCompile(`^[a-zA-Z0-9]{64}$`)
)

type tokenCredential struct {
	token string
}

func NewTokenCredential(token string) credentials.PerRPCCredentials {
	return &tokenCredential{token: token}
}

func (s tokenCredential) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	return map[string]string{
		TokenKey: s.token,
	}, nil
}

func (s tokenCredential) RequireTransportSecurity() bool {
	return false
}

func TokenFromIncomingContext(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Errorf(codes.Unauthenticated, "missing metadata")
	}

	routerKey := md[TokenKey]
	if len(routerKey) == 0 {
		return "", status.Errorf(codes.Unauthenticated, "missing %s", TokenKey)
	}

	return routerKey[0], nil
}

// ValidateKey validates a router key. It will return nil if the key is exactly length 64 and only contains
// alphanumeric characters.
func ValidateKey(k string) error {
	if !routerKeyRegexp.MatchString(k) {
		return eris.Errorf("invalid %s, must be length 64 and only contain alphanumerics", TokenKey)
	}
	return nil
}
