package sidecar

import (
	"testing"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gotest.tools/assert"

	sidecar "github.com/argus-labs/argus/sidecar/v1"
)

func GetBankClient(t *testing.T, url string) banktypes.QueryClient {
	conn, err := grpc.Dial(url, grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NilError(t, err)
	return banktypes.NewQueryClient(conn)
}

func GetSidecarClient(t *testing.T, url string) sidecar.SidecarClient {
	conn, err := grpc.Dial(url, grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NilError(t, err)
	return sidecar.NewSidecarClient(conn)
}
