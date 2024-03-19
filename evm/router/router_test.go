package router

import (
	"context"
	"math/big"
	"testing"

	"cosmossdk.io/log"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	types2 "github.com/ethereum/go-ethereum/core/types"
	"gotest.tools/v3/assert"
	"pkg.berachain.dev/polaris/eth/core/types"

	namespacetypes "pkg.world.dev/world-engine/evm/x/namespace/types"
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
	contractAddr := common.HexToAddress("0x61d2B2315605660c3855C8BE139B82e0635E13E3")
	namespace, msgID, msg := "cardinal", "tx1", []byte("hello")
	// queue a message
	err := router.SendMessage(context.Background(), "foobar", namespace, contractAddr.String(), msgID, msg)
	assert.NilError(t, err)
	// make sure its set in the queue
	assert.Equal(t, router.queue.IsSet(contractAddr), true)
	tx := types.NewTransaction(
		1,
		contractAddr,
		big.NewInt(10),
		40,
		big.NewInt(10),
		[]byte("hello"),
	)
	txHash := tx.Hash()
	// test dispatch when there is a successful tx
	router.PostBlockHook(types.Transactions{tx}, types.Receipts{
		&types.Receipt{
			Status: types2.ReceiptStatusSuccessful,
			TxHash: txHash,
		},
	}, nil)
	// queue should be cleared after dispatching
	assert.Equal(t, router.queue.IsSet(contractAddr), false)
}
