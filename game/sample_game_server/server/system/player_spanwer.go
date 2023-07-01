package system

import (
	"github.com/argus-labs/world-engine/cardinal/ecs"
	comp "github.com/argus-labs/world-engine/game/sample_game_server/server/component"
	tx "github.com/argus-labs/world-engine/game/sample_game_server/server/transaction"
)

func PlayerSpawnerSystem(world *ecs.World, tq *ecs.TransactionQueue) error {
	createTxs := tx.CreatePlayer.In(tq)
	newPlayerIDs, err := world.CreateMany(len(createTxs), comp.Health, comp.Position)
	if err != nil {
		return err
	}
	for i := range createTxs {
		id := newPlayerIDs[i]
		createTx := createTxs[i]
		if err := comp.Health.Set(world, id, comp.HealthComponent{Val: 100}); err != nil {
			return err
		}
		if err := comp.Position.Set(world, id, comp.PositionComponent{
			X: createTx.X,
			Y: createTx.Y,
		}); err != nil {
			return err
		}
	}
	return nil
}
