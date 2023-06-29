package tests

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/argus-labs/world-engine/cardinal/chain/mocks"
	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/inmem"
	"github.com/argus-labs/world-engine/cardinal/ecs/transaction"
)

type SendEnergy struct {
	To, From string
	Amount   uint64
}

func TestWorld_WithChain(t *testing.T) {
	ctrl := gomock.NewController(t)
	adapter := mocks.NewMockAdapter(ctrl)
	w := inmem.NewECSWorldForTest(t, ecs.WithAdapter(adapter))

	sendEnergyTx := ecs.NewTransactionType[SendEnergy]()
	txId := transaction.TypeID(1)
	err := sendEnergyTx.SetID(txId)
	assert.NilError(t, err)

	txToSend := SendEnergy{"You", "Me", 400}

	w.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) error {
		return nil
	})

	expectedBatch := ecs.TxBatch{
		TxID: txId,
		Txs:  []any{txToSend},
	}
	expectedBatches := []ecs.TxBatch{expectedBatch}

	bz, err := json.Marshal(expectedBatches)
	assert.NilError(t, err)

	adapter.EXPECT().Submit(gomock.Any(), bz).Times(1)

	err = w.LoadGameState()
	assert.NilError(t, err)

	sendEnergyTx.AddToQueue(w, txToSend)
	err = w.Tick()
	assert.NilError(t, err)

	// sleep to let the go routine finish handling the mock tx submission.
	// without this, the test returns without ever waiting for the mock to signal that it's done.
	time.Sleep(time.Millisecond * 1)
}
