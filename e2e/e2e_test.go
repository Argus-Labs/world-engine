package e2e

import (
	"context"
	"fmt"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/chain/x/shard/types"
	"testing"
	"time"
)

const (
	adminNakamaID = "00000000-0000-0000-0000-000000000000"
)

type ClaimPersonaTx struct {
	PersonaTag string `json:"persona_tag"`
}

type Empty struct{}

var emptyStruct = Empty{}

type MoveTx struct {
	Direction string `json:"Direction"`
}

func TestTransactionStoredOnChain(t *testing.T) {
	c := newClient(t)
	chain := NewChainClient(t)
	user := "foooooobar"
	persona := "fooooobar"
	err := c.registerDevice(user, adminNakamaID)
	assert.NilError(t, err)

	_, err = c.rpc("nakama/claim-persona", ClaimPersonaTx{PersonaTag: persona})
	assert.NilError(t, err)

	time.Sleep(time.Second * 2)

	_, err = c.rpc("tx-join", emptyStruct)
	assert.NilError(t, err)
	time.Sleep(2 * time.Second)

	_, err = c.rpc("tx-move", MoveTx{Direction: "up"})
	assert.NilError(t, err)
	time.Sleep(2 * time.Second)

	txs, err := chain.shard.Transactions(context.Background(), &types.QueryTransactionsRequest{
		Namespace: "TESTGAME",
		Page:      nil,
	})
	assert.NilError(t, err)
	fmt.Println(txs.String())
}

func TestCanGRPCQueryChain(t *testing.T) {
	chain := NewChainClient(t)
	res, err := chain.bank.Params(context.Background(), &banktypes.QueryParamsRequest{})
	assert.NilError(t, err)
	fmt.Printf("%+v\n", res)
}
