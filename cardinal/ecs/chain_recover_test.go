package ecs_test

import (
	"context"
	"encoding/binary"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/ecs/message"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"sort"
	"testing"

	"google.golang.org/protobuf/proto"
	"gotest.tools/v3/assert"
	shardv1 "pkg.world.dev/world-engine/rift/shard/v1"

	"github.com/cometbft/cometbft/libs/rand"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/shard"
	"pkg.world.dev/world-engine/chain/x/shard/types"
	"pkg.world.dev/world-engine/sign"
)

var _ shard.Adapter = &DummyAdapter{}

type DummyAdapter struct {
	txs map[uint64][]*types.Transaction
}

func (d *DummyAdapter) Submit(_ context.Context, p *sign.Transaction, txID, tick uint64) error {
	sp := &shardv1.Transaction{
		PersonaTag: p.PersonaTag,
		Namespace:  p.Namespace,
		Nonce:      p.Nonce,
		Signature:  p.Signature,
		Body:       p.Body,
	}
	bz, err := proto.Marshal(sp)
	if err != nil {
		return err
	}
	if d.txs[tick] == nil {
		d.txs[tick] = make([]*types.Transaction, 0)
	}
	d.txs[tick] = append(d.txs[tick], &types.Transaction{
		TxId:                 txID,
		GameShardTransaction: bz,
	})
	return nil
}

func (d *DummyAdapter) QueryTransactions(_ context.Context, request *types.QueryTransactionsRequest,
) (*types.QueryTransactionsResponse, error) {
	tickedTxs := make([]*types.Epoch, 0, len(d.txs))
	for tick, txs := range d.txs {
		tickedTxs = append(tickedTxs, &types.Epoch{
			Epoch: tick,
			Txs:   txs,
		})
	}
	sort.Slice(tickedTxs, func(i, j int) bool {
		return tickedTxs[i].Epoch < tickedTxs[j].Epoch
	})

	var pr *types.PageResponse
	if request.Page.Key == nil {
		half := len(tickedTxs) / 2
		tickedTxs = tickedTxs[0:half]
		nextKey := make([]byte, 8)
		binary.BigEndian.PutUint64(nextKey, uint64(half))
		pr = &types.PageResponse{Key: nextKey}
	} else {
		key := binary.BigEndian.Uint64(request.Page.Key)
		tickedTxs = tickedTxs[key:]
		pr = nil
	}

	return &types.QueryTransactionsResponse{
		Epochs: tickedTxs,
		Page:   pr,
	}, nil
}

type SendEnergyMsg struct {
	To, From string
	Amount   uint64
}

type SendEnergyResult struct{}

// TestWorld_RecoverFromChain tests that after submitting transactions to the chain, they can be queried, re-ran,
// and end up with the same game state as before.
func TestWorld_RecoverFromChain(t *testing.T) {
	// setup world and transactions
	ctx := context.Background()
	adapter := &DummyAdapter{txs: make(map[uint64][]*types.Transaction, 0)}
	w := testutils.NewTestWorld(t, cardinal.WithAdapter(adapter)).Instance()
	sendEnergyTx := ecs.NewMessageType[SendEnergyMsg, SendEnergyResult]("send_energy")
	err := w.RegisterMessages(sendEnergyTx)
	assert.NilError(t, err)

	sysRuns := uint64(0)
	timesSendEnergyRan := 0
	// send energy system
	w.RegisterSystem(func(wCtx ecs.WorldContext) error {
		sysRuns++
		txs := sendEnergyTx.In(wCtx)
		if len(txs) > 0 {
			timesSendEnergyRan++
		}
		return nil
	})
	namespace := "game1"
	payloads := make([]*sign.Transaction, 0, 10)
	var finalTick uint64 = 20
	for i := 0; i <= 10; i++ {
		payload := generateRandomTransaction(t, namespace, sendEnergyTx)
		payloads = append(payloads, payload)
		err = adapter.Submit(ctx, payload, uint64(sendEnergyTx.ID()), uint64(i+i)) // final tick should be 10+10 = 20
		assert.NilError(t, err)
	}

	err = w.LoadGameState()
	assert.NilError(t, err)
	err = w.RecoverFromChain(ctx)
	assert.NilError(t, err)
	assert.Equal(t, finalTick, w.CurrentTick()-1) // the current tick should be 1 minus the last tick processed.
	assert.Equal(t, sysRuns, w.CurrentTick())
	assert.Equal(t, len(payloads), timesSendEnergyRan)
}

func generateRandomTransaction(t *testing.T, ns string, msg message.Message) *sign.Transaction {
	tx1 := SendEnergyMsg{
		To:     rand.Str(5),
		From:   rand.Str(4),
		Amount: rand.Uint64(),
	}
	bz, err := msg.Encode(tx1)
	assert.NilError(t, err)
	return &sign.Transaction{
		PersonaTag: rand.Str(5),
		Namespace:  ns,
		Nonce:      rand.Uint64(),
		Signature:  rand.Str(10),
		Body:       bz,
	}
}

func TestWorld_RecoverShouldErrorIfTickExists(t *testing.T) {
	ctx := context.Background()
	adapter := &DummyAdapter{}
	w := testutils.NewTestWorld(t, cardinal.WithAdapter(adapter)).Instance()
	assert.NilError(t, w.LoadGameState())
	assert.NilError(t, w.Tick(ctx))

	err := w.RecoverFromChain(ctx)
	assert.ErrorContains(t, err, "world recovery should not occur in a world with existing state")
}
