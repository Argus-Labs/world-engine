package tests

import (
	"context"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/argus-labs/world-engine/cardinal/chain"
	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/inmem"
	"github.com/argus-labs/world-engine/chain/x/shard/types"
)

var _ chain.Adapter = &DummyAdapter{}

type DummyAdapter struct {
	batches []*types.TransactionBatch
}

func (d *DummyAdapter) Submit(ctx context.Context, namespace string, tick uint64, txs []byte) error {
	d.batches = append(d.batches, &types.TransactionBatch{
		Namespace: namespace,
		Tick:      tick,
		Batch:     txs,
	})
	return nil
}

func (d *DummyAdapter) QueryBatch(ctx context.Context, req *types.QueryBatchesRequest) (*types.QueryBatchesResponse, error) {
	return &types.QueryBatchesResponse{
		Batches: d.batches,
		Page:    nil,
	}, nil
}

type SendEnergyTransaction struct {
	To, From string
	Amount   uint64
}

type ClaimPlanetTransaction struct {
	Claimant string
	PlanetID uint64
}

// TestWorld_RecoverFromChain tests that after submitting transactions to the chain, they can be queried, re-ran,
// and end up with the same game state as before.
func TestWorld_RecoverFromChain(t *testing.T) {
	// setup world and transactions
	ctx := context.Background()
	adapter := &DummyAdapter{batches: make([]*types.TransactionBatch, 0)}
	w := inmem.NewECSWorldForTest(t, ecs.WithAdapter(adapter))
	SendEnergyTx := ecs.NewTransactionType[SendEnergyTransaction]()
	ClaimPlanetTx := ecs.NewTransactionType[ClaimPlanetTransaction]()
	err := w.RegisterTransactions(SendEnergyTx, ClaimPlanetTx)
	assert.NilError(t, err)

	// setup the transactions we will "recover" from the chain (dummy adapter).
	sendEnergyTxs := []SendEnergyTransaction{
		{
			"rogue4",
			"warrior3",
			920,
		},
		{
			"mage1",
			"ranger3",
			40,
		},
	}
	claimPlanetTxs := []ClaimPlanetTransaction{
		{"mage1", 32509235},
	}

	sendEnergyTimesRan := 0
	claimPlanetTimesRan := 0
	// SendEnergySystem simply checks that the transactions received in the queue match the ones we submit above.
	w.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) error {
		sendEnergyTimesRan++
		txs := SendEnergyTx.In(queue)
		for i, tx := range txs {
			assert.DeepEqual(t, tx, sendEnergyTxs[i])
		}
		assert.Equal(t, len(txs), len(sendEnergyTxs))
		return nil
	})

	// ClaimPlanetSystem simply checks that the transactions received in the queue match the ones we submit above.
	w.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) error {
		claimPlanetTimesRan++
		txs := ClaimPlanetTx.In(queue)
		for i, tx := range txs {
			assert.DeepEqual(t, tx, claimPlanetTxs[i])
		}
		assert.Equal(t, len(txs), len(claimPlanetTxs))
		return nil
	})
	assert.NilError(t, w.LoadGameState())

	// add the transactions to the world queue.
	for _, tx := range sendEnergyTxs {
		SendEnergyTx.AddToQueue(w, tx)
	}
	for _, tx := range claimPlanetTxs {
		ClaimPlanetTx.AddToQueue(w, tx)
	}

	// we want to run a tick, so that the transactions go through the submission process, and end up stored in our
	// dummy adapter above.
	doneSignal := make(chan struct{})
	ctx = context.WithValue(ctx, "done", doneSignal)
	err = w.Tick(ctx)
	assert.NilError(t, err)
	select {
	case <-doneSignal:
		break
	}

	// now we can recover, which will run the same transactions we submitted before, as they are now stored in
	// the dummy adapter, and will be run again.
	err = w.RecoverFromChain(ctx)
	assert.NilError(t, err)
	select {
	case <-doneSignal:
		break
	}
	// ensure the systems ran twice. once for the tick above, and then again for the recovery.
	assert.Equal(t, sendEnergyTimesRan, 2)
	assert.Equal(t, claimPlanetTimesRan, 2)
}
