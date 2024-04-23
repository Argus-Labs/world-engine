package system

import (
	"fmt"

	"github.com/ByteArena/box2d"
	comp "github.com/argus-labs/world-engine/example/tester/agarbenchmark/component"
	"github.com/argus-labs/world-engine/example/tester/agarbenchmark/physics"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
)

var isPlayerRecoverySystemInitialized = false

func PlayerRecoveryInitSystem(wCtx cardinal.WorldContext) error {
	if isPlayerRecoverySystemInitialized {
		return nil
	}

	if wCtx.CurrentTick() == 0 {
		wCtx.Logger().Info().Msgf("PlayerRecoveryInitSystem: This is tick 0, so there's nothing to recover.")
		isPlayerRecoverySystemInitialized = true
	}

	numPlayersRecovered := 0

	err := cardinal.NewSearch().
		Entity(filter.Contains(filter.Component[comp.Player]())).
		Each(wCtx, func(entityID types.EntityID) bool {
			wCtx.Logger().Info().Msgf("PlayerRecoveryInitSystem: Entity ID: %d", entityID)

			player, err := cardinal.GetComponent[comp.Player](wCtx, entityID)
			if err != nil {
				DebugLogError(wCtx, "Failed to get player component:", err)
				return true
			}
			linearVelocity, err := cardinal.GetComponent[comp.LinearVelocity](wCtx, entityID)
			if err != nil {
				DebugLogError(wCtx, "Failed to get linear velocity component:", err)
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
			body := physics.Instance().CreatePlayerBody(entityID, p, radius.Length)
			v := box2d.MakeB2Vec2(linearVelocity.X, linearVelocity.Y)
			body.SetLinearVelocity(v)
			body.SetTransform(p, 0)

			SendSpawnPlayer(wCtx, entityID, player.PersonaTag, p)
			numPlayersRecovered++

			wCtx.Logger().Info().Msgf("PlayerRecoveryInitSystem: Recovered player %v at (%f, %f) EntityID: %d radius: %f velocity: (%f, %f)", player.PersonaTag, p.X, p.Y, entityID, radius.Length, v.X, v.Y)

			return true
		})
	if err != nil {
		return fmt.Errorf("PlayerRecoveryInitSystem: %w", err)
	}

	if (numPlayersRecovered > 0) && !isPlayerRecoverySystemInitialized {
		isPlayerRecoverySystemInitialized = true
		wCtx.Logger().Info().Msgf("PlayerRecoveryInitSystem: Recovered %d players.", numPlayersRecovered)
	}

	return nil
}
