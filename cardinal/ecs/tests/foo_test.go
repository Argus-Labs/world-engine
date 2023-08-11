package tests

import (
	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	"testing"
)

func TestThing(t *testing.T) {
	type Health struct {
		Amount uint64
		Cap    uint64
	}
	HealthComponent := ecs.NewComponentType[Health]()

	type AttackInput struct {
		TargetPlayer uint64
		Amount       uint64
	}

	type AttackOutput struct {
		Success bool
	}
	AttackTx := ecs.NewTransactionType[AttackInput, AttackOutput]("attack")

	AttackTx.SetResult()

	var attackSystem ecs.System = func(world *ecs.World, queue *ecs.TransactionQueue) error {
		txs := AttackTx.In(queue)

		for _, tx := range txs {
			atk := tx.Value
			err := HealthComponent.Update(world, storage.EntityID(atk.TargetPlayer), func(health Health) Health {
				health.Amount -= atk.Amount
				return health
			})
			if err != nil {
				AttackTx.SetResult(world, tx.ID, AttackOutput{Success: false})
				AttackTx.AddError(world, tx.ID, err)
				continue
			}
			AttackTx.SetResult(world, tx.ID, AttackOutput{Success: true})
		}
		return nil
	}
}
