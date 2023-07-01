package system

import (
	"github.com/argus-labs/world-engine/cardinal/ecs"
	comp "github.com/argus-labs/world-engine/game/sample_game/server/component"
	tx "github.com/argus-labs/world-engine/game/sample_game/server/transaction"
)

func FireSpawnerSystem(world *ecs.World, tq *ecs.TransactionQueue) error {
	for _, createFire := range tx.CreateFire.In(tq) {
		id, err := world.Create(comp.Position)
		if err != nil {
			return err
		}
		if err = comp.Position.Set(world, id, comp.PositionComponent{X: createFire.X, Y: createFire.Y}); err != nil {
			return err
		}
	}
	return nil
}
