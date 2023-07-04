package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

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

func TestWorld_RecoverFromChain(t *testing.T) {
	ctx := context.Background()
	// namespace := "dark_forest1"
	adapter := &DummyAdapter{batches: make([]*types.TransactionBatch, 0)}
	w := inmem.NewECSWorldForTest(t, ecs.WithAdapter(adapter))
	SendEnergyTx := ecs.NewTransactionType[SendEnergyTransaction]()
	ClaimPlanetTx := ecs.NewTransactionType[ClaimPlanetTransaction]()
	err := w.RegisterTransactions(SendEnergyTx, ClaimPlanetTx)
	assert.NilError(t, err)

	timesSendEnergySystemRan := 0
	timesClaimPlanetSystemRan := 0

	// SendEnergySystem
	w.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) error {
		txs := SendEnergyTx.In(queue)
		if len(txs) > 0 {
			timesSendEnergySystemRan++
		}
		return nil
	})

	// ClaimPlanetSystem
	w.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) error {
		txs := ClaimPlanetTx.In(queue)
		if len(txs) > 0 {
			timesClaimPlanetSystemRan++
		}
		return nil
	})

	assert.NilError(t, w.LoadGameState())

	SendEnergyTx.AddToQueue(w, SendEnergyTransaction{
		To:     "player1",
		From:   "player2",
		Amount: 40000,
	})
	SendEnergyTx.AddToQueue(w, SendEnergyTransaction{
		To:     "mage1",
		From:   "ranger3",
		Amount: 9910,
	})

	ClaimPlanetTx.AddToQueue(w, ClaimPlanetTransaction{
		Claimant: "mage1",
		PlanetID: 93202352,
	})

	err = w.Tick()
	assert.NilError(t, err)
	time.Sleep(3 * time.Second)
	//send1Bz, err := SendEnergyTx.Encode(SendEnergyTransaction{
	//	To:     "player1",
	//	From:   "player2",
	//	Amount: 40000,
	//})
	//assert.NilError(t, err)
	//
	//send2Bz, err := SendEnergyTx.Encode(SendEnergyTransaction{
	//	To:     "dark_mage1",
	//	From:   "light_mage2",
	//	Amount: 300,
	//})
	//assert.NilError(t, err)
	//
	//claimPlanetBz, err := ClaimPlanetTx.Encode(ClaimPlanetTransaction{
	//	Claimant: "mage1",
	//	PlanetID: 92359235,
	//})
	//assert.NilError(t, err)
	//
	//batch1 := []*ecs.TxBatch{
	//	{
	//		TxID: SendEnergyTx.ID(),
	//		Txs:  []any{send1Bz, send2Bz},
	//	},
	//}
	//batch1Bz, err := json.Marshal(batch1)
	//assert.NilError(t, err)
	//
	//batch2 := []ecs.TxBatch{
	//	{
	//		TxID: ClaimPlanetTx.ID(),
	//		Txs:  []any{claimPlanetBz},
	//	},
	//}
	//batch2Bz, err := json.Marshal(batch2)
	//assert.NilError(t, err)
	//
	//assert.NilError(t, adapter.Submit(ctx, namespace, 10, batch1Bz))
	//assert.NilError(t, adapter.Submit(ctx, namespace, 11, batch2Bz))

	err = w.RecoverFromChain(ctx)
	assert.NilError(t, err)
	fmt.Println(timesSendEnergySystemRan)
	fmt.Println(timesClaimPlanetSystemRan)
}

func TestEncoding(t *testing.T) {
	send1 := SendEnergyTransaction{
		To:     "foo",
		From:   "bar",
		Amount: 42,
	}
	send2 := SendEnergyTransaction{
		To:     "nar",
		From:   "foo",
		Amount: 22,
	}

	batches := []*ecs.TxBatch{
		{
			TxID: 1,
			Txs:  []any{send1, send2},
		},
	}

	bz, err := json.Marshal(batches)
	assert.NilError(t, err)
	fmt.Println(string(bz))

	var newTxBatches []*ecs.TxBatch
	err = json.Unmarshal(bz, &newTxBatches)
	assert.NilError(t, err)

	fmt.Println(newTxBatches[0].Txs)
}
