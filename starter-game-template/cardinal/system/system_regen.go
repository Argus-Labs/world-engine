package system

import (
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"

	comp "github.com/argus-labs/starter-game-template/cardinal/component"
)

// RegenSystem replenishes the player's HP at every tick.
// This provides an example of a system that doesn't rely on a transaction to update a component.
func RegenSystem(world cardinal.WorldContext) error {
	return cardinal.NewSearch(world, filter.Exact(comp.Player{}, comp.Health{})).Each(func(id types.EntityID) bool {
		health, err := cardinal.GetComponent[comp.Health](world, id)
		if err != nil {
			return true
		}
		health.HP++
		if err := cardinal.SetComponent[comp.Health](world, id, health); err != nil {
			return true
		}
		return true
	})
}
