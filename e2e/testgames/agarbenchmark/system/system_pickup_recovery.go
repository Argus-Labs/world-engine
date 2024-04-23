package system

import (
	"fmt"

	"github.com/ByteArena/box2d"
	comp "github.com/argus-labs/world-engine/example/tester/agarbenchmark/component"
	"github.com/argus-labs/world-engine/example/tester/agarbenchmark/physics"
	"github.com/argus-labs/world-engine/example/tester/agarbenchmark/world"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
)

var isPickupRecoverySystemInitialized = false

func PickupRecoveryInitSystem(wCtx cardinal.WorldContext) error {
	if isPickupRecoverySystemInitialized {
		return nil
	}

	if wCtx.CurrentTick() == 0 {
		wCtx.Logger().Info().Msgf("PickupRecoveryInitSystem: This is tick 0, so there's nothing to recover.")
		isPickupRecoverySystemInitialized = true
	}

	numPickupsRecovered := 0

	err := cardinal.NewSearch().
		Entity(filter.Contains(filter.Component[comp.Pickup]())).
		Each(wCtx, func(entityID types.EntityID) bool {
			wCtx.Logger().Info().Msgf("PickupRecoveryInitSystem: Entity ID: %d", entityID)

			pickup, err := cardinal.GetComponent[comp.Pickup](wCtx, entityID)
			if err != nil {
				DebugLogError(wCtx, "Failed to get pickup component:", err)
				return true
			}
			position, err := cardinal.GetComponent[comp.Position](wCtx, entityID)
			if err != nil {
				DebugLogError(wCtx, "Failed to get position component:", err)
				return true
			}
			radius, err := cardinal.GetComponent[comp.Radius](wCtx, entityID)
			if err != nil {
				DebugLogError(wCtx, "Failed to get radius component:", err)
				return true
			}

			p := box2d.MakeB2Vec2(position.X, position.Y)
			_ = physics.Instance().CreatePickupBody(entityID, p, radius.Length)
			numPickupsRecovered++

			if pickup.ID == world.CoinPickup {
				wCtx.Logger().Info().Msgf("PickupRecoveryInitSystem: Recovered coin pickup at (%f, %f) EntityID: %d radius: %f", p.X, p.Y, entityID, radius.Length)
			} else if pickup.ID == world.MedpackPickup {
				wCtx.Logger().Info().Msgf("PickupRecoveryInitSystem: Recovered medpack pickup at (%f, %f) EntityID: %d radius: %f", p.X, p.Y, entityID, radius.Length)
			}
			return true
		})
	if err != nil {
		return fmt.Errorf("PickupRecoveryInitSystem: %w", err)
	}

	if (numPickupsRecovered > 0) && !isPickupRecoverySystemInitialized {
		isPickupRecoverySystemInitialized = true
		wCtx.Logger().Info().Msgf("PickupRecoveryInitSystem: Recovered %d pickups.", numPickupsRecovered)
	}

	return nil
}
