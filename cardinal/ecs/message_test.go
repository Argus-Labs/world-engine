package ecs_test

import (
	"context"
	"errors"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"testing"

	"pkg.world.dev/world-engine/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/internal/testutil"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types/message"
)

func TestForEachTransaction(t *testing.T) {
	eng := testutils.NewTestFixture(t, nil).Engine
	type SomeMsgRequest struct {
		GenerateError bool
	}
	type SomeMsgResponse struct {
		Successful bool
	}

	someMsg := ecs.NewMessageType[SomeMsgRequest, SomeMsgResponse]("some_msg")
	assert.NilError(t, eng.RegisterMessages(someMsg))

	err := eng.RegisterSystems(func(eCtx engine.Context) error {
		someMsg.Each(eCtx, func(t ecs.TxData[SomeMsgRequest]) (result SomeMsgResponse, err error) {
			if t.Msg.GenerateError {
				return result, errors.New("some error")
			}
			return SomeMsgResponse{
				Successful: true,
			}, nil
		})
		return nil
	})
	assert.NilError(t, err)
	assert.NilError(t, eng.LoadGameState())

	// Add 10 transactions to the tx queue and keep track of the hashes that we just created
	knownTxHashes := map[message.TxHash]SomeMsgRequest{}
	for i := 0; i < 10; i++ {
		req := SomeMsgRequest{GenerateError: i%2 == 0}
		txHash := someMsg.AddToQueue(eng, req, testutil.UniqueSignature(t))
		knownTxHashes[txHash] = req
	}

	// Perform a engine tick
	assert.NilError(t, eng.Tick(context.Background()))

	// Verify the receipts for the previous tick are what we expect
	receipts, err := eng.GetTransactionReceiptsForTick(eng.CurrentTick() - 1)
	assert.NilError(t, err)
	assert.Equal(t, len(knownTxHashes), len(receipts))
	for _, receipt := range receipts {
		request, ok := knownTxHashes[receipt.TxHash]
		assert.Check(t, ok)
		if request.GenerateError {
			assert.Check(t, len(receipt.Errs) > 0)
		} else {
			assert.Equal(t, 0, len(receipt.Errs))
			assert.Equal(t, receipt.Result.(SomeMsgResponse), SomeMsgResponse{Successful: true})
		}
	}
}
