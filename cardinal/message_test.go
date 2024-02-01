package cardinal_test

import (
	"context"
	"errors"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/cardinal/types/entity"
	"pkg.world.dev/world-engine/cardinal/types/message"
	"testing"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"

	"pkg.world.dev/world-engine/cardinal"
)

type AddHealthToEntityTx struct {
	TargetID entity.ID
	Amount   int
}

type AddHealthToEntityResult struct{}

var addHealthToEntity = cardinal.NewMessageType[AddHealthToEntityTx, AddHealthToEntityResult]("add_health")

func TestTransactionExample(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world, doTick := tf.World, tf.DoTick
	assert.NilError(t, cardinal.RegisterComponent[Health](world))
	assert.NilError(t, cardinal.RegisterMessages(world, addHealthToEntity))
	err := cardinal.RegisterSystems(world, func(eCtx engine.Context) error {
		// test "In" method
		for _, tx := range addHealthToEntity.In(eCtx) {
			targetID := tx.Msg.TargetID
			err := cardinal.UpdateComponent[Health](eCtx, targetID, func(h *Health) *Health {
				h.Value = tx.Msg.Amount
				return h
			})
			assert.Check(t, err == nil)
		}
		// test same as above but with forEach
		addHealthToEntity.Each(eCtx,
			func(tx cardinal.TxData[AddHealthToEntityTx]) (AddHealthToEntityResult, error) {
				targetID := tx.Msg.TargetID
				err := cardinal.UpdateComponent[Health](eCtx, targetID, func(h *Health) *Health {
					h.Value = tx.Msg.Amount
					return h
				})
				assert.Check(t, err == nil)
				return AddHealthToEntityResult{}, errors.New("fake tx error")
			})

		return nil
	})
	assert.NilError(t, err)

	testWorldCtx := testutils.WorldToEngineContext(world)
	doTick()
	ids, err := cardinal.CreateMany(testWorldCtx, 10, Health{})
	assert.NilError(t, err)

	// Queue up the transaction.
	idToModify := ids[3]
	amountToModify := 20
	payload := testutils.UniqueSignature()
	testutils.AddTransactionToWorldByAnyTransaction(
		world, addHealthToEntity,
		AddHealthToEntityTx{idToModify, amountToModify}, payload,
	)

	// The health change should be applied during this tick
	doTick()

	// Make sure the target entity had its health updated.
	for _, id := range ids {
		var health *Health
		health, err = cardinal.GetComponent[Health](testWorldCtx, id)
		assert.NilError(t, err)
		if id == idToModify {
			assert.Equal(t, amountToModify, health.Value)
		} else {
			assert.Equal(t, 0, health.Value)
		}
	}
	// Make sure transaction errors are recorded in the receipt
	receipts, err := testWorldCtx.GetTransactionReceiptsForTick(testWorldCtx.CurrentTick() - 1)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(receipts))
	assert.Equal(t, 1, len(receipts[0].Errs))
}

func TestForEachTransaction(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World
	type SomeMsgRequest struct {
		GenerateError bool
	}
	type SomeMsgResponse struct {
		Successful bool
	}

	someMsg := cardinal.NewMessageType[SomeMsgRequest, SomeMsgResponse]("some_msg")
	assert.NilError(t, cardinal.RegisterMessages(world, someMsg))

	err := cardinal.RegisterSystems(world, func(eCtx engine.Context) error {
		someMsg.Each(eCtx, func(t cardinal.TxData[SomeMsgRequest]) (result SomeMsgResponse, err error) {
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
	assert.NilError(t, world.LoadGameState())

	// Add 10 transactions to the tx queue and keep track of the hashes that we just cardinal.Created
	knownTxHashes := map[message.TxHash]SomeMsgRequest{}
	for i := 0; i < 10; i++ {
		req := SomeMsgRequest{GenerateError: i%2 == 0}
		txHash := someMsg.AddToQueue(world, req, testutils.UniqueSignature())
		knownTxHashes[txHash] = req
	}

	// Perform a engine tick
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
			assert.Equal(t, receipt.Result.(SomeMsgResponse), SomeMsgResponse{Successful: true})
		}
	}
}
