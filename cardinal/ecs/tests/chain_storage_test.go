package tests

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/argus-labs/world-engine/cardinal/chain"
	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/inmem"
	"github.com/argus-labs/world-engine/chain/x/shard/types"
)

type SendEnergy struct {
	To, From string
	Amount   uint64
}

var _ chain.Adapter = &MockAdapter{}

type MockAdapter struct {
	lastSubmittedValue []byte
	timesCalled        int
	done               chan int
}

func (m *MockAdapter) QueryBatch(ctx context.Context, req *types.QueryBatchesRequest) (*types.QueryBatchesResponse, error) {
	panic("not implemented")
}

var sg sync.WaitGroup

func (m *MockAdapter) Submit(ctx context.Context, ns string, tick uint64, bz []byte) error {
	m.lastSubmittedValue = bz
	m.timesCalled++
	sg.Done()
	return nil
}

func TestWorld_WithChain(t *testing.T) {
	sg = sync.WaitGroup{}
	mockAdapter := &MockAdapter{}
	w := inmem.NewECSWorldForTest(t, ecs.WithAdapter(mockAdapter))

	sendEnergyTx := ecs.NewTransactionType[SendEnergy]()
	err := w.RegisterTransactions(sendEnergyTx)
	assert.NilError(t, err)

	txToSend := SendEnergy{"You", "Me", 400}

	w.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) error {
		return nil
	})

	expectedBatch := ecs.TxBatch{
		TxID: sendEnergyTx.ID(),
		Txs:  []any{txToSend},
	}
	expectedBatches := []ecs.TxBatch{expectedBatch}

	bz, err := json.Marshal(expectedBatches)
	assert.NilError(t, err)

	err = w.LoadGameState()
	assert.NilError(t, err)

	sendEnergyTx.AddToQueue(w, txToSend)
	sg.Add(1)
	err = w.Tick(context.Background())
	assert.NilError(t, err)

	sg.Wait()
	assert.Equal(t, mockAdapter.timesCalled, 1)
	assert.DeepEqual(t, mockAdapter.lastSubmittedValue, bz)

}
