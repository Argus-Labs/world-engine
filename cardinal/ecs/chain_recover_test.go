package ecs_test

import (
	"context"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"math"
	"pkg.world.dev/world-engine/cardinal/txpool"
	"pkg.world.dev/world-engine/cardinal/types/entity"
	shardv2 "pkg.world.dev/world-engine/rift/shard/v2"
	"testing"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/shard"
	"pkg.world.dev/world-engine/evm/x/shard/types"
)

var _ shard.Adapter = &DummyAdapter{}

type DummyAdapter struct {
	ticks []*types.Epoch
}

func newAdapter(ticks []*types.Epoch) *DummyAdapter {
	return &DummyAdapter{ticks: ticks}
}

func (d *DummyAdapter) Submit(_ context.Context, _ txpool.TxMap, _ string, _ uint64, _ uint64) error {
	panic("not implemented")
}

func (d *DummyAdapter) QueryTransactions(_ context.Context, _ *types.QueryTransactionsRequest,
) (*types.QueryTransactionsResponse, error) {
	return &types.QueryTransactionsResponse{
		Epochs: d.ticks,
		Page:   nil,
	}, nil
}

type IncreaseEnergy struct {
	Amount uint64 `json:"amount"`
}

type IncreaseEnergyResult struct{}

type EnergyComp struct {
	Amount uint64
}

func (e EnergyComp) Name() string {
	return "energy-comp"
}

// TestWorld_RecoverFromChain tests that after submitting transactions to the chain, they can be queried, re-ran,
// and end up with the same game state as before.
func TestWorld_RecoverFromChain(t *testing.T) {
	tx1 := shardv2.Transaction{
		PersonaTag: "ty",
		Namespace:  cardinal.DefaultNamespace,
		Nonce:      1,
		Signature:  "foo",
		Body:       json.RawMessage(`{"amount": 10}`),
	}
	tx1Bz, err := proto.Marshal(&tx1)
	assert.NilError(t, err)
	tx2 := shardv2.Transaction{
		PersonaTag: "ty",
		Namespace:  cardinal.DefaultNamespace,
		Nonce:      2,
		Signature:  "foo2",
		Body:       json.RawMessage(`{"amount": 5}`),
	}
	tx2Bz, err := proto.Marshal(&tx2)
	assert.NilError(t, err)

	ticks := make([]*types.Epoch, 0)
	epoch1 := &types.Epoch{
		Epoch:         1,
		UnixTimestamp: 120,
		Txs: []*types.Transaction{
			{
				TxId:                 3,
				GameShardTransaction: tx1Bz,
			},
		},
	}
	epoch2 := &types.Epoch{
		Epoch:         2,
		UnixTimestamp: 121,
		Txs: []*types.Transaction{
			{
				TxId:                 3,
				GameShardTransaction: tx2Bz,
			},
		},
	}
	ticks = append(ticks, epoch1, epoch2)

	adapter := newAdapter(ticks)
	eng := testutils.NewTestWorld(t, cardinal.WithAdapter(adapter)).Engine()
	increaseEnergyTx := ecs.NewMessageType[IncreaseEnergy, IncreaseEnergyResult]("send_energy")
	err = eng.RegisterMessages(increaseEnergyTx)
	assert.NilError(t, err)
	err = ecs.RegisterComponent[EnergyComp](eng)
	assert.NilError(t, err)

	var compID entity.ID = math.MaxUint64
	sys := func(ctx ecs.EngineContext) error {
		increaseEnergyTx.Each(ctx, func(tx ecs.TxData[IncreaseEnergy]) (IncreaseEnergyResult, error) {
			if compID == math.MaxUint64 {
				id, err := ecs.Create(ctx, EnergyComp{tx.Msg.Amount})
				assert.NilError(t, err)
				compID = id
			} else {
				err = ecs.UpdateComponent[EnergyComp](ctx, compID, func(e *EnergyComp) *EnergyComp {
					e.Amount += tx.Msg.Amount
					return e
				})
				assert.NilError(t, err)
			}
			return IncreaseEnergyResult{}, nil
		})
		return nil
	}
	eng.RegisterSystem(sys)

	err = eng.LoadGameState()
	assert.NilError(t, err)
	err = eng.RecoverFromChain(context.Background())
	assert.NilError(t, err)

	energy, err := ecs.GetComponent[EnergyComp](ecs.NewEngineContext(eng), compID)
	assert.NilError(t, err)

	// energy should be 15 since we the transactions that came in were 5 and 10.
	assert.Equal(t, energy.Amount, uint64(15))
	assert.Equal(t, eng.CurrentTick(), uint64(3)) // current tick should be 3 as chain only returned up to 2.
}

func TestWorld_RecoverShouldErrorIfTickExists(t *testing.T) {
	ctx := context.Background()
	adapter := &DummyAdapter{}
	eng := testutils.NewTestWorld(t, cardinal.WithAdapter(adapter)).Engine()
	assert.NilError(t, eng.LoadGameState())
	assert.NilError(t, eng.Tick(ctx))

	err := eng.RecoverFromChain(ctx)
	assert.ErrorContains(t, err, "world recovery should not occur in a world with existing state")
}
