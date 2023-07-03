package system

import (
	"github.com/argus-labs/world-engine/cardinal/ecs"
	comp "github.com/argus-labs/world-engine/game/sample_game/server/component"
	tx "github.com/argus-labs/world-engine/game/sample_game/server/transaction"
)

func MoveSystem(world *ecs.World, tq *ecs.TransactionQueue) error {
	for _, move := range tx.Move.In(tq) {
		pos, err := comp.Position.Get(world, move.ID)
		if err != nil {
			return err
		}
		pos.X += move.XDelta
		pos.Y += move.YDelta
		if err = comp.Position.Set(world, move.ID, pos); err != nil {
			return err
		}
	}
	return nil
}
