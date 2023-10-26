package e2e

import (
	"context"
	"fmt"
	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/chain/x/shard/types"
	"testing"
	"time"
)

const (
	adminNakamaID = "00000000-0000-0000-0000-000000000000"
)

type ClaimPersonaTx struct {
	PersonaTag string `json:"personaTag"`
}

type Empty struct{}

var emptyStruct = Empty{}

type MoveTx struct {
	Direction string `json:"direction"`
}

func TestTransactionStoredOnChain(t *testing.T) {
	t.Skip("do not run this test unless the docker containers for base shard, game shard, nakama, and " +
		"celestia are running. to run the stack, run `make game` and `make rollup`. wait for startup and run this test")
	c := newClient(t)
	chain := newChainClient(t)
	user := "foo"
	persona := "foobar"
	err := c.registerDevice(user, adminNakamaID)
	assert.NilError(t, err)

	res, err := c.rpc("nakama/claim-persona", ClaimPersonaTx{PersonaTag: persona})
	assert.NilError(t, err)

	fmt.Printf("%+v\n", res)
	assert.Equal(t, 200, res.StatusCode, "claim persona failed with code %d: body: %v", res.StatusCode, res.Body)
	time.Sleep(time.Second * 3)

	res, err = c.rpc("nakama/show-persona", ClaimPersonaTx{PersonaTag: persona})
	assert.NilError(t, err)

	fmt.Printf("%+v\n", res)
	assert.Equal(t, 200, res.StatusCode, "show persona failed with code %d: body: %v", res.StatusCode, res.Body)
	time.Sleep(time.Second * 3)

	res, err = c.rpc("tx/game/join", emptyStruct)
	assert.NilError(t, err)
	assert.Equal(t, 200, res.StatusCode, "tx-join failed with code %d: body %v", res.StatusCode, res.Body)
	time.Sleep(2 * time.Second)

	res, err = c.rpc("tx/game/move", MoveTx{Direction: "up"})
	assert.NilError(t, err)
	assert.Equal(t, 200, res.StatusCode, "tx- failed with code %d: body %v", res.StatusCode, res.Body)
	time.Sleep(2 * time.Second)

	txs, err := chain.shard.Transactions(context.Background(), &types.QueryTransactionsRequest{
		Namespace: "TESTGAME",
		Page:      nil,
	})
	assert.NilError(t, err)
	assert.Check(t, len(txs.Epochs) != 0)
}
