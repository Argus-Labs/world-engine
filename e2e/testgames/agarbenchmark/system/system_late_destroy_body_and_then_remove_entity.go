package system

import (
	"fmt"

	"github.com/argus-labs/world-engine/example/tester/agarbenchmark/physics"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/types"
)

// Used as a hashset to prevent any chance of duplicate removals.
var pendingRemovals map[types.EntityID]struct{}

// NOTE: This is only intended for entities with rigid bodies.
func LateDestroyBodyAndThenRemoveEntitySystem(wCtx cardinal.WorldContext) error {
	for entityID := range pendingRemovals {
		wCtx.Logger().Info().Msgf("LateDestroyBodyAndThenRemoveEntitySystem: Destroying body for EntityID: %d", entityID)
		err := physics.Instance().DestroyBody(entityID)
		if err != nil {
			panic(fmt.Sprintf("LateDestroyBodyAndThenRemoveEntitySystem: Failed to destroy body for EntityID: %d", entityID))
		}
		wCtx.Logger().Info().Msgf("LateDestroyBodyAndThenRemoveEntitySystem: Removing EntityID: %d", entityID)
		err = cardinal.Remove(wCtx, entityID)
		if err != nil {
			panic(err)
		}
		wCtx.Logger().Info().Msgf("LateDestroyBodyAndThenRemoveEntitySystem: Successfully cleaned up EntityID: %d", entityID)
	}
	// Clear the "hashset" after all removals have finished.
	pendingRemovals = make(map[types.EntityID]struct{})
	return nil
}

func SchedulePhysicalEntityForLateRemoval(entityID types.EntityID) {
	_, ok := physics.Instance().TryGetBody(entityID)
	if !ok {
		panic(fmt.Sprintf("SchedulePhysicalEntityForLateRemoval: EntityID %d does not exist in the physics system", entityID))
	}
	pendingRemovals[entityID] = struct{}{}
}
