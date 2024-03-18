package gamestate_test

import (
	"context"
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/txpool"
)

func TestCanSaveAndRecoverTransactions(t *testing.T) {
	type MsgIn struct {
		Value int
	}
	type MsgOut struct {
		Value int
	}

	msgAlpha := testutils.NewMessageType[MsgIn, MsgOut]("alpha")
	msgBeta := testutils.NewMessageType[MsgIn, MsgOut]("beta")
	assert.NilError(t, msgAlpha.SetID(16))
	assert.NilError(t, msgBeta.SetID(32))
	msgs := []types.Message{msgAlpha, msgBeta}

	manager, client := newCmdBufferAndRedisClientForTest(t, nil)
	originalPool := txpool.New()
	sig := testutils.UniqueSignature()
	_ = originalPool.AddTransaction(msgAlpha.ID(), MsgIn{100}, sig)

	assert.NilError(t, manager.StartNextTick(msgs, originalPool))

	// Pretend some problem was encountered here. Make sure we can recover the transactions from redis.
	manager, _ = newCmdBufferAndRedisClientForTest(t, client)
	gotPool, err := manager.Recover(msgs)
	assert.NilError(t, err)

	assert.Equal(t, gotPool.GetAmountOfTxs(), originalPool.GetAmountOfTxs())

	// Make sure we can finalize the tick
	assert.NilError(t, manager.StartNextTick(msgs, gotPool))
	assert.NilError(t, manager.FinalizeTick(context.Background()))
}

func TestErrorWhenRecoveringNoTransactions(t *testing.T) {
	manager := newCmdBufferForTest(t)
	_, err := manager.Recover(nil)
	// Recover should fail when no transactions have previously been saved to the DB.
	assert.Check(t, err != nil)
}
