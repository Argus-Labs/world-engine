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

	sendEnergyTransactionsSeen := 0
	claimPlanetTransactionsSeen := 0

	// SendEnergySystem
	w.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) error {
		txs := SendEnergyTx.In(queue)
		sendEnergyTransactionsSeen = len(txs)
		return nil
	})

	// ClaimPlanetSystem
	w.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) error {
		txs := ClaimPlanetTx.In(queue)
		claimPlanetTransactionsSeen = len(txs)
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
	time.Sleep(1 * time.Second)

	err = w.RecoverFromChain(ctx)
	fmt.Println(sendEnergyTransactionsSeen)
	fmt.Println(claimPlanetTransactionsSeen)
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

	for _, batch := range newTxBatches {
		for _, tx := range batch.Txs {
			bz, err := json.Marshal(tx)
			assert.NilError(t, err)
			var sendTx SendEnergyTransaction
			err = json.Unmarshal(bz, &sendTx)
			assert.NilError(t, err)
			fmt.Printf("%+v", sendTx)
		}
		fmt.Println("TxID: ", batch.TxID)
	}
}
