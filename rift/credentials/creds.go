package credentials

import (
	"context"
	"regexp"

	"github.com/rotisserie/eris"
	"google.golang.org/grpc/credentials"
)

var (
	TokenKey = "router_key"

	_ credentials.PerRPCCredentials = &simpleTokenCredential{}

	routerKeyRegexp = regexp.MustCompile(`^[a-zA-Z0-9]{32}$`)
)

type simpleTokenCredential struct {
	token string
}

func NewSimpleTokenCredential(token string) credentials.PerRPCCredentials {
	return &simpleTokenCredential{token: token}
}

func (s simpleTokenCredential) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	return map[string]string{
		TokenKey: s.token,
	}, nil
}

func (s simpleTokenCredential) RequireTransportSecurity() bool {
	return false
}

func ValidateKey(k string) error {
	if !routerKeyRegexp.MatchString(k) {
		return eris.Errorf("invalid %s, must be length 32 and only contain alphanumerics", TokenKey)
	}
	return nil
}
