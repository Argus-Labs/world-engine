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
	tick    uint64
	batches []*types.TransactionBatch
}

func (d *DummyAdapter) Submit(ctx context.Context, namespace string, tick uint64, txs []byte) error {
	d.batches = append(d.batches, &types.TransactionBatch{
		Namespace: namespace,
		Tick:      d.tick,
		Batch:     txs,
	})
	return nil
}

func (d *DummyAdapter) QueryBatches(ctx context.Context, req *types.QueryBatchesRequest) (*types.QueryBatchesResponse, error) {
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
	// CAR-96: This test is currently broken. See:
	// https://linear.app/arguslabs/issue/CAR-96/testworld-recoverfromchain-fail-on-main
	//t.Skip()

	// setup world and transactions
	ctx := context.Background()
	adapter := &DummyAdapter{batches: make([]*types.TransactionBatch, 0), tick: 30}
	w := inmem.NewECSWorldForTest(t, ecs.WithAdapter(adapter))
	SendEnergyTx := ecs.NewTransactionType[SendEnergyTransaction]("send_energy")
	ClaimPlanetTx := ecs.NewTransactionType[ClaimPlanetTransaction]("claim_planet")
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
	sendEnergySys := func(world *ecs.World, queue *ecs.TransactionQueue) error {
		sendEnergyTimesRan++
		txs := SendEnergyTx.In(queue)
		for i, tx := range txs {
			assert.DeepEqual(t, tx, sendEnergyTxs[i])
		}
		assert.Equal(t, len(txs), len(sendEnergyTxs))
		return nil
	}
	// SendEnergySystem simply checks that the transactions received in the queue match the ones we submit above.
	w.AddSystem(sendEnergySys)

	claimPlanetSys := func(world *ecs.World, queue *ecs.TransactionQueue) error {
		claimPlanetTimesRan++
		txs := ClaimPlanetTx.In(queue)
		for i, tx := range txs {
			assert.DeepEqual(t, tx, claimPlanetTxs[i])
		}
		assert.Equal(t, len(txs), len(claimPlanetTxs))
		return nil
	}
	// ClaimPlanetSystem simply checks that the transactions received in the queue match the ones we submit above.
	w.AddSystem(claimPlanetSys)
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
	<-doneSignal

	w2 := inmem.NewECSWorldForTest(t, ecs.WithAdapter(adapter))
	assert.NilError(t, w2.RegisterTransactions(SendEnergyTx, ClaimPlanetTx))
	w2.AddSystem(sendEnergySys)
	w2.AddSystem(claimPlanetSys)
	assert.NilError(t, w2.LoadGameState())

	// now we can recover, which will run the same transactions we submitted before, as they are now stored in
	// the dummy adapter, and will be run again.
	err = w2.RecoverFromChain(ctx)
	// ensure the systems ran twice. once for the tick above, and then again for the recovery.
	assert.Equal(t, sendEnergyTimesRan, 2)
	assert.Equal(t, claimPlanetTimesRan, 2)

	// ensure that the tick was updated from the stored transaction batch.
	assert.Equal(t, adapter.tick+1, uint64(w2.CurrentTick()))
}

func TestWorld_RecoverShouldErrorIfTickExists(t *testing.T) {
	// setup world and transactions
	ctx := context.Background()
	adapter := &DummyAdapter{batches: make([]*types.TransactionBatch, 0), tick: 30}
	w := inmem.NewECSWorldForTest(t, ecs.WithAdapter(adapter))
	assert.NilError(t, w.LoadGameState())
	assert.NilError(t, w.Tick(ctx))

	err := w.RecoverFromChain(ctx)
	assert.ErrorContains(t, err, "world recovery should not occur in a world with existing state")
}
