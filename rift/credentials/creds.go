package credentials

import (
	"context"

	"google.golang.org/grpc/credentials"
)

var TokenKey = "router_key"

var _ credentials.PerRPCCredentials = &simpleTokenCredential{}

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
