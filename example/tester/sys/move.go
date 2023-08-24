package sys

import (
	"fmt"
	"github.com/argus-labs/world-engine/example/tester/comp"
	"github.com/argus-labs/world-engine/example/tester/tx"
	"pkg.world.dev/world-engine/cardinal/ecs"
)

func Move(world *ecs.World, queue *ecs.TransactionQueue) error {
	for _, mtx := range tx.MoveTx.In(queue) {
		playerEntityID, ok := PlayerEntityID[mtx.Sig.PersonaTag]
		if !ok {
			tx.MoveTx.AddError(world, mtx.ID, fmt.Errorf("player %s has not joined yet", mtx.Sig.PersonaTag))
		}
		err := comp.LocationComponent.Update(world, playerEntityID, func(location comp.Location) comp.Location {
			switch mtx.Value.Direction {
			case "up":
				location.Y += 1
			case "down":
				location.Y -= 1
			case "left":
				location.X -= 1
			case "right":
				location.X += 1
			}
			return location
		})
		if err != nil {
			tx.MoveTx.AddError(world, mtx.ID, err)
		}
	}
	return nil
}
