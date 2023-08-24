package sys

import (
	"github.com/argus-labs/world-engine/example/tester/comp"
	"github.com/argus-labs/world-engine/example/tester/tx"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

var PlayerEntityID = make(map[string]storage.EntityID, 0)

func Join(world *ecs.World, queue *ecs.TransactionQueue) error {
	for _, jtx := range tx.JoinTx.In(queue) {
		entity, err := world.Create(comp.LocationComponent, comp.PlayerComponent)
		if err != nil {
			tx.JoinTx.AddError(world, jtx.ID, err)
			continue
		}
		err = comp.PlayerComponent.Update(world, entity, func(player comp.Player) comp.Player {
			player.Name = jtx.Sig.PersonaTag
			return player
		})
		if err != nil {
			tx.JoinTx.AddError(world, jtx.ID, err)
			continue
		}
		PlayerEntityID[jtx.Sig.PersonaTag] = entity
	}
	return nil
}
