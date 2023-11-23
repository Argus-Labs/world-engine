package ecb_test

import (
	"testing"

	"pkg.world.dev/world-engine/cardinal/ecs/message"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/internal/testutil"
)

func TestCanSaveAndRecoverTransactions(t *testing.T) {
	type MsgIn struct {
		Value int
	}
	type MsgOut struct {
		Value int
	}

	msgAlpha := ecs.NewMessageType[MsgIn, MsgOut]("alpha")
	msgBeta := ecs.NewMessageType[MsgIn, MsgOut]("beta")
	assert.NilError(t, msgAlpha.SetID(16))
	assert.NilError(t, msgBeta.SetID(32))
	msgs := []message.Message{msgAlpha, msgBeta}

	manager, client := newCmdBufferAndRedisClientForTest(t, nil)
	originalQueue := message.NewTxQueue()
	sig := testutil.UniqueSignature(t)
	_ = originalQueue.AddTransaction(msgAlpha.ID(), MsgIn{100}, sig)

	assert.NilError(t, manager.StartNextTick(msgs, originalQueue))

	// Pretend some problem was encountered here. Make sure we can recover the transactions from redis.
	manager, _ = newCmdBufferAndRedisClientForTest(t, client)
	gotQueue, err := manager.Recover(msgs)
	assert.NilError(t, err)

	assert.Equal(t, gotQueue.GetAmountOfTxs(), originalQueue.GetAmountOfTxs())

	// Make sure we can finalize the tick
	assert.NilError(t, manager.StartNextTick(msgs, gotQueue))
	assert.NilError(t, manager.FinalizeTick())
}

func TestErrorWhenRecoveringNoTransactions(t *testing.T) {
	manager := newCmdBufferForTest(t)
	_, err := manager.Recover(nil)
	// Recover should fail when no transactions have previously been saved to the DB.
	assert.Check(t, err != nil)
}
