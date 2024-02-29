package chain

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	namespacetypes "pkg.world.dev/world-engine/evm/x/namespace/types"
	routerv1 "pkg.world.dev/world-engine/rift/router/v1"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/evm/x/shard/types"
)

const (
	adminNakamaID = "00000000-0000-0000-0000-000000000000"
)

type ClaimPersona struct {
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
	c := newClient()
	chain := newChainClient(t)
	user := "foo"
	persona := "foobar"
	err := c.registerDevice(user, adminNakamaID)
	assert.NilError(t, err)

	res, err := c.rpc("nakama/claim-persona", ClaimPersona{PersonaTag: persona})
	assert.NilError(t, err)

	assert.Equal(t, 200, res.StatusCode, "claim persona failed with code %d: body: %v", res.StatusCode, res.Body)
	time.Sleep(time.Second * 3)

	res, err = c.rpc("nakama/show-persona", ClaimPersona{PersonaTag: persona})
	assert.NilError(t, err)

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

// TestNamespaceSaved ensures that when the stack is running, namespaces can be queried
// and the address in the namespace can be supplied to a router msg client and send requests.
func TestNamespaceSaved(t *testing.T) {
	t.Skip("do not run this test unless the docker containers for base shard, game shard, nakama, and " +
		"celestia are running. to run the stack, run `make game` and `make rollup`. wait for startup and run this test")
	chain := newChainClient(t)
	res, err := chain.namespace.Namespaces(context.Background(), &namespacetypes.NamespacesRequest{})
	assert.NilError(t, err)
	assert.Equal(t, len(res.Namespaces), 1)
	ns := res.Namespaces[0]
	addr := ns.ShardAddress

	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NilError(t, err)
	client := routerv1.NewMsgClient(conn)
	_, err = client.QueryShard(context.Background(), &routerv1.QueryShardRequest{Request: []byte("nah")})
	// if we got any sort of contextual error message back, we know that Cardinal received this request.
	assert.ErrorContains(t, err, "query with name  not found")
}
