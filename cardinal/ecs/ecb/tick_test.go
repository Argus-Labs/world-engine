package ecb_test

import (
	"testing"

	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/internal/testutil"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
)

func TestCanSaveAndRecoverTransactions(t *testing.T) {
	type TxIn struct {
		Value int
	}
	type TxOut struct {
		Value int
	}

	txAlpha := ecs.NewTransactionType[TxIn, TxOut]("alpha")
	txBeta := ecs.NewTransactionType[TxIn, TxOut]("beta")
	assert.NilError(t, txAlpha.SetID(16))
	assert.NilError(t, txBeta.SetID(32))
	txs := []transaction.ITransaction{txAlpha, txBeta}

	manager, client := newCmdBufferAndRedisClientForTest(t, nil)
	originalQueue := transaction.NewTxQueue()
	sig := testutil.UniqueSignature(t)
	_ = originalQueue.AddTransaction(txAlpha.ID(), TxIn{100}, sig)

	assert.NilError(t, manager.StartNextTick(txs, originalQueue))

	// Pretend some problem was encountered here. Make sure we can recover the transactions from redis.
	manager, _ = newCmdBufferAndRedisClientForTest(t, client)
	gotQueue, err := manager.Recover(txs)
	assert.NilError(t, err)

	assert.Equal(t, gotQueue.GetAmountOfTxs(), originalQueue.GetAmountOfTxs())

	// Make sure we can finalize the tick
	assert.NilError(t, manager.StartNextTick(txs, gotQueue))
	assert.NilError(t, manager.FinalizeTick())
}

func TestErrorWhenRecoveringNoTransactions(t *testing.T) {
	manager := newCmdBufferForTest(t)
	_, err := manager.Recover(nil)
	// Recover should fail when no transactions have previously been saved to the DB.
	assert.Check(t, err != nil)
}
