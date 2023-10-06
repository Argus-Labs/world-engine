package ecs_test

import (
	"context"
	"errors"
	"testing"

	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/internal/testutil"
	"pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
)

func TestForEachTransaction(t *testing.T) {
	world := ecs.NewTestWorld(t)
	type SomeTxRequest struct {
		GenerateError bool
	}
	type SomeTxResponse struct {
		Successful bool
	}

	someTx := ecs.NewTransactionType[SomeTxRequest, SomeTxResponse]("some_tx")
	assert.NilError(t, world.RegisterTransactions(someTx))

	world.AddSystem(func(world *ecs.World, queue *transaction.TxQueue, logger *log.Logger) error {
		someTx.ForEach(world, queue, func(t ecs.TxData[SomeTxRequest]) (result SomeTxResponse, err error) {
			if t.Value.GenerateError {
				return result, errors.New("some error")
			}
			return SomeTxResponse{
				Successful: true,
			}, nil
		})
		return nil
	})
	assert.NilError(t, world.LoadGameState())

	// Add 10 transactions to the tx queue and keep track of the hashes that we just created
	knownTxHashes := map[transaction.TxHash]SomeTxRequest{}
	for i := 0; i < 10; i++ {
		req := SomeTxRequest{GenerateError: i%2 == 0}
		txHash := someTx.AddToQueue(world, req, testutil.UniqueSignature(t))
		knownTxHashes[txHash] = req
	}

	// Perform a world tick
	assert.NilError(t, world.Tick(context.Background()))

	// Verify the receipts for the previous tick are what we expect
	receipts, err := world.GetTransactionReceiptsForTick(world.CurrentTick() - 1)
	assert.NilError(t, err)
	assert.Equal(t, len(knownTxHashes), len(receipts))
	for _, receipt := range receipts {
		request, ok := knownTxHashes[receipt.TxHash]
		assert.Check(t, ok)
		if request.GenerateError {
			assert.Check(t, len(receipt.Errs) > 0)
		} else {
			assert.Equal(t, 0, len(receipt.Errs))
			assert.Equal(t, receipt.Result.(SomeTxResponse), SomeTxResponse{Successful: true})
		}
	}
}
