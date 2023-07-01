package system

import (
	"log"

	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	comp "github.com/argus-labs/world-engine/game/sample_game/server/component"
)

func BurnSystem(world *ecs.World, tq *ecs.TransactionQueue) error {
	fires := map[comp.PositionComponent]bool{}
	ecs.NewQuery(filter.Exact(comp.Position)).Each(world, func(id storage.EntityID) {
		pos, err := comp.Position.Get(world, id)
		if err != nil {
			log.Print(err)
			return
		}
		fires[pos] = true
	})
	ecs.NewQuery(filter.Exact(comp.Health, comp.Position)).Each(world, func(id storage.EntityID) {
		pos, err := comp.Position.Get(world, id)
		if err != nil {
			log.Print(err)
			return
		}
		if !fires[pos] {
			return
		}

		health, err := comp.Health.Get(world, id)
		if err != nil {
			log.Print(err)
			return
		}
		health.Val -= 10
		if err := comp.Health.Set(world, id, health); err != nil {
			log.Print(err)
			return
		}
	})
	return nil
}
