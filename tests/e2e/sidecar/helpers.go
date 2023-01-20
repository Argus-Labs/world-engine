package sidecar

import (
	"testing"

	g1 "buf.build/gen/go/argus-labs/argus/grpc/go/v1/sidecarv1grpc"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gotest.tools/assert"

	adaptertypesv1 "github.com/argus-labs/argus/x/adapter/types/v1"
)

func GetBankClient(t *testing.T, url string) banktypes.QueryClient {
	conn, err := grpc.Dial(url, grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NilError(t, err)
	return banktypes.NewQueryClient(conn)
}

func GetSidecarClient(t *testing.T, url string) g1.SidecarClient {
	conn, err := grpc.Dial(url, grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NilError(t, err)
	return g1.NewSidecarClient(conn)
}

func GetAdapterClient(t *testing.T, url string) adaptertypesv1.QueryClient {
	conn, err := grpc.Dial(url, grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NilError(t, err)
	return adaptertypesv1.NewQueryClient(conn)
}
