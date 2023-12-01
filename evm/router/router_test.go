package router

/*
import (
	"context"
	"errors"
	"math/big"
	"testing"

	"gotest.tools/v3/assert"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"pkg.berachain.dev/polaris/eth/core/types"
	namespacetypes "pkg.world.dev/world-engine/evm/x/namespace/types"

	"cosmossdk.io/log"
)

func mockQueryCtx(_ int64, _ bool) (sdk.Context, error) {
	return sdk.Context{}, nil
}

func mockGetAddr(_ context.Context, _ *namespacetypes.AddressRequest) (*namespacetypes.AddressResponse, error) {
	return &namespacetypes.AddressResponse{Address: "localhost:9090"}, nil
}

func TestRouter(t *testing.T) {
	r := NewRouter(log.NewTestLogger(t), mockQueryCtx, mockGetAddr)
	router, ok := r.(*routerImpl)
	assert.Equal(t, ok, true)

	namespace, sender, msgID, msg := "cardinal", "foo", "tx1", []byte("hello")
	// queue a message
	err := router.SendMessage(context.Background(), namespace, sender, msgID, msg)
	assert.NilError(t, err)
	// make sure its set in the queue
	assert.Equal(t, router.queue.IsSet(), true)
	tx := types.NewTransaction(
		1,
		common.HexToAddress("0x61d2B2315605660c3855C8BE139B82e0635E13E3"),
		big.NewInt(10),
		40,
		big.NewInt(10),
		[]byte("hello"),
	)
	// test dispatch when there is a successful tx
	router.PostBlockHook(tx, &core.ExecutionResult{Err: nil})
	// queue should be cleared after dispatching
	assert.Equal(t, router.queue.IsSet(), false)

	// queue another message
	err = router.SendMessage(context.Background(), namespace, sender, msgID, msg)
	assert.NilError(t, err)

	// this time, lets check when the execution Result is failed, we still clear the queue.
	router.PostBlockHook(tx, &core.ExecutionResult{Err: errors.New("some error")})
	assert.Equal(t, router.queue.IsSet(), false)
}
*/
