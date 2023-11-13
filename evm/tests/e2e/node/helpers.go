package node

import (
	"testing"

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
