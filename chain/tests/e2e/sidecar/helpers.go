package sidecar

import (
	"testing"

	"buf.build/gen/go/argus-labs/world-engine/grpc/go/sidecar/v1/sidecarv1grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gotest.tools/assert"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

func GetBankClient(t *testing.T, url string) banktypes.QueryClient {
	conn, err := grpc.Dial(url, grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NilError(t, err)
	return banktypes.NewQueryClient(conn)
}

func GetSidecarClient(t *testing.T, url string) sidecarv1grpc.SidecarClient {
	conn, err := grpc.Dial(url, grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NilError(t, err)
	return sidecarv1grpc.NewSidecarClient(conn)
}
