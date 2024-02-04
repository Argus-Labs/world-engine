package system

import (
	comp "github.com/argus-labs/starter-game-template/cardinal/component"
	"pkg.world.dev/world-engine/cardinal"
)

// RegenSystem replenishes the player's HP at every tick.
// This provides an example of a system that doesn't rely on a transaction to update a component.
func RegenSystem(world cardinal.WorldContext) error {
	search := world.NewSearch(cardinal.Exact(comp.Player{}, comp.Health{}))

	err := search.Each(world, func(id cardinal.EntityID) bool {
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
	if err != nil {
		return err
	}

	return nil
}
