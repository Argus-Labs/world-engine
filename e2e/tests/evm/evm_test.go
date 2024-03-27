package evm

import (
	"context"
	"crypto/rand"
	"io"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/e2e/tests/clients"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gotest.tools/v3/assert"

	namespacetypes "pkg.world.dev/world-engine/evm/x/namespace/types"
	"pkg.world.dev/world-engine/evm/x/shard/types"
	routerv1 "pkg.world.dev/world-engine/rift/router/v1"
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
	c := clients.NewNakamaClient(t)
	chain := clients.NewEVMClient(t)
	user := randomString()
	persona := randomString()
	err := c.RegisterDevice(user, generateRandomNakamaID())
	assert.NilError(t, err)

	res, err := c.RPC("nakama/claim-persona", ClaimPersona{PersonaTag: persona})
	assert.NilError(t, err)

	bodyBytes, err := io.ReadAll(res.Body)
	assert.NilError(t, err)
	assert.Equal(t, 200, res.StatusCode, "claim persona failed with code %d: body: %s", res.StatusCode,
		string(bodyBytes))
	time.Sleep(time.Second * 3)

	res, err = c.RPC("nakama/show-persona", ClaimPersona{PersonaTag: persona})
	assert.NilError(t, err)

	assert.Equal(t, 200, res.StatusCode, "show persona failed with code %d: body: %v", res.StatusCode, res.Body)
	time.Sleep(time.Second * 3)

	res, err = c.RPC("tx/game/join", emptyStruct)
	assert.NilError(t, err)
	assert.Equal(t, 200, res.StatusCode, "tx-join failed with code %d: body %v", res.StatusCode, res.Body)
	time.Sleep(2 * time.Second)

	res, err = c.RPC("tx/game/move", MoveTx{Direction: "up"})
	assert.NilError(t, err)
	assert.Equal(t, 200, res.StatusCode, "tx- failed with code %d: body %v", res.StatusCode, res.Body)
	time.Sleep(2 * time.Second)

	txs, err := chain.Shard.Transactions(context.Background(), &types.QueryTransactionsRequest{
		Namespace: "TESTGAME",
		Page:      nil,
	})
	assert.NilError(t, err)
	assert.Check(t, len(txs.Epochs) != 0)
}

// TestNamespaceSaved ensures that when the stack is running, namespaces can be queried
// and the address in the namespace can be supplied to a router msg client and send requests.
func TestNamespaceSaved(t *testing.T) {
	chain := clients.NewEVMClient(t)
	res, err := chain.Namespace.Namespaces(context.Background(), &namespacetypes.NamespacesRequest{})
	assert.NilError(t, err)
	assert.Equal(t, len(res.Namespaces), 1)
	ns := res.Namespaces[0]
	addr := ns.ShardAddress

	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NilError(t, err)
	client := routerv1.NewMsgClient(conn)
	_, err = client.QueryShard(context.Background(), &routerv1.QueryShardRequest{Request: []byte("nah")})
	// if we got any sort of contextual error message back, we know that Cardinal received this request.
	assert.ErrorContains(t, err, `is not registered`)
}

func randomString() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	sb := strings.Builder{}
	sb.Grow(10)
	for i := 0; i < 10; i++ {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		sb.WriteByte(alphabet[idx.Int64()])
	}
	return sb.String()
}

func generateRandomNakamaID() string {
	const format = "00000000-0000-0000-0000-000000000000"
	const digits = "0123456789"

	result := make([]byte, len(format))
	for i := 0; i < len(format); i++ {
		if format[i] != '-' {
			idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
			result[i] = digits[idx.Int64()]
		} else {
			result[i] = '-'
		}
	}
	return string(result)
}
