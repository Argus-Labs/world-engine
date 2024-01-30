package cardinal_test

import (
	"errors"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"testing"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"

	"pkg.world.dev/world-engine/cardinal"
)

type AddHealthToEntityTx struct {
	TargetID cardinal.EntityID
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
			func(tx ecs.TxData[AddHealthToEntityTx]) (AddHealthToEntityResult, error) {
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
