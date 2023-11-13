package cardinal_test

import (
	"errors"
	"testing"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal"
)

type AddHealthToEntityTx struct {
	TargetID cardinal.EntityID
	Amount   int
}

type AddHealthToEntityResult struct{}

var addHealthToEntity = cardinal.NewMessageType[AddHealthToEntityTx, AddHealthToEntityResult]("add_health")

func TestApis(t *testing.T) {
	// this test just makes sure certain signatures remain the same.
	// If they change this test will trigger a compiler error.
	x := cardinal.TxData[Alpha]{}
	x.Tx()
	x.Hash()
	assert.Equal(t, x.Msg().Name(), "alpha")
	type randoTx struct{}
	type randoTxResult struct{}
	cardinal.NewMessageTypeWithEVMSupport[randoTx, randoTxResult]("rando_with_evm")
}

func TestTransactionExample(t *testing.T) {
	world, doTick := testutils.MakeWorldAndTicker(t)
	assert.NilError(t, cardinal.RegisterComponent[Health](world))
	assert.NilError(t, cardinal.RegisterMessages(world, addHealthToEntity))
	err := cardinal.RegisterSystems(world, func(worldCtx cardinal.WorldContext) error {
		// test "In" method
		for _, tx := range addHealthToEntity.In(worldCtx) {
			targetID := tx.Msg().TargetID
			err := cardinal.UpdateComponent[Health](worldCtx, targetID, func(h *Health) *Health {
				h.Value = tx.Msg().Amount
				return h
			})
			assert.Check(t, err == nil)
		}
		// test same as above but with forEach
		addHealthToEntity.ForEach(worldCtx, func(tx cardinal.TxData[AddHealthToEntityTx]) (AddHealthToEntityResult, error) {
			targetID := tx.Msg().TargetID
			err := cardinal.UpdateComponent[Health](worldCtx, targetID, func(h *Health) *Health {
				h.Value = tx.Msg().Amount
				return h
			})
			assert.Check(t, err == nil)
			addHealthToEntity.AddError(worldCtx, tx.Hash(), errors.New("test error"))
			// redundant but for testing purposes
			addHealthToEntity.SetResult(worldCtx, tx.Hash(), AddHealthToEntityResult{})
			_, errs, ok := addHealthToEntity.GetReceipt(worldCtx, tx.Hash()) // check if receipts are working.
			assert.Assert(t, ok)
			assert.Equal(t, len(errs), 1)
			return AddHealthToEntityResult{}, nil
		})

		addHealthToEntity.Convert() // Check for compilation error

		return nil
	})
	assert.NilError(t, err)

	testWorldCtx := testutils.WorldToWorldContext(world)
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
}
