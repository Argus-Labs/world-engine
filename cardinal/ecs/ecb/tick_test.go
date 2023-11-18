package ecb_test

import (
	"testing"

	"pkg.world.dev/world-engine/cardinal/ecs/internal/ecstestutils"
	"pkg.world.dev/world-engine/cardinal/ecs/message"
	"pkg.world.dev/world-engine/cardinal/testutils"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
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
	testutils.AssertNilErrorWithTrace(t, msgAlpha.SetID(16))
	testutils.AssertNilErrorWithTrace(t, msgBeta.SetID(32))
	msgs := []message.Message{msgAlpha, msgBeta}

	manager, client := newCmdBufferAndRedisClientForTest(t, nil)
	originalQueue := message.NewTxQueue()
	sig := ecstestutils.UniqueSignature(t)
	_ = originalQueue.AddTransaction(msgAlpha.ID(), MsgIn{100}, sig)

	testutils.AssertNilErrorWithTrace(t, manager.StartNextTick(msgs, originalQueue))

	// Pretend some problem was encountered here. Make sure we can recover the transactions from redis.
	manager, _ = newCmdBufferAndRedisClientForTest(t, client)
	gotQueue, err := manager.Recover(msgs)
	testutils.AssertNilErrorWithTrace(t, err)

	assert.Equal(t, gotQueue.GetAmountOfTxs(), originalQueue.GetAmountOfTxs())

	// Make sure we can finalize the tick
	testutils.AssertNilErrorWithTrace(t, manager.StartNextTick(msgs, gotQueue))
	testutils.AssertNilErrorWithTrace(t, manager.FinalizeTick())
}

func TestErrorWhenRecoveringNoTransactions(t *testing.T) {
	manager := newCmdBufferForTest(t)
	_, err := manager.Recover(nil)
	// Recover should fail when no transactions have previously been saved to the DB.
	assert.Check(t, err != nil)
}
