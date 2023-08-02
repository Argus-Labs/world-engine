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
	ecs.NewQuery(filter.Exact(comp.Position)).Each(world, func(id storage.EntityID) bool {
		pos, err := comp.Position.Get(world, id)
		if err != nil {
			log.Print(err)
			return true
		}
		fires[pos] = true
		return true
	})
	ecs.NewQuery(filter.Exact(comp.Health, comp.Position)).Each(world, func(id storage.EntityID) bool {
		pos, err := comp.Position.Get(world, id)
		if err != nil {
			log.Print(err)
			return true
		}
		if !fires[pos] {
			return true
		}

		health, err := comp.Health.Get(world, id)
		if err != nil {
			log.Print(err)
			return true
		}
		health.Val -= 10
		if err := comp.Health.Set(world, id, health); err != nil {
			log.Print(err)
			return true
		}
		return true
	})
	return nil
}
