package system

import (
	"fmt"

	comp "github.com/argus-labs/world-engine/example/tester/agarbenchmark/component"
	"github.com/argus-labs/world-engine/example/tester/agarbenchmark/msg"
	"github.com/argus-labs/world-engine/example/tester/agarbenchmark/physics"
	"github.com/argus-labs/world-engine/example/tester/agarbenchmark/world"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/message"
)

// PlayerSpawnerSystem spawns players based on `CreatePlayer` transactions.
// This provides an example of a system that creates a new entity.
func PlayerSpawnerSystem(wCtx cardinal.WorldContext) error {
	return cardinal.EachMessage(
		wCtx,
		func(create message.TxData[msg.CreatePlayerMsg]) (msg.CreatePlayerResult, error) {
			spawnpoint := getRandomGridPosition()
			id, err := cardinal.Create(wCtx,
				comp.Player{PersonaTag: create.Msg.TargetPersonaTag},
				comp.Health{Value: world.Settings.InitialPlayerHealth()},
				comp.Level{Value: 0},
				comp.Score{Value: 0},
				comp.Wealth{Value: 0},

				comp.Bearing{Degrees: 0},
				comp.LinearVelocity{X: 0, Y: 0},
				comp.Position{X: spawnpoint.X, Y: spawnpoint.Y},
				comp.Radius{Length: world.Settings.PlayerRadius()},
				comp.RigidBody{IsStatic: false, IsSensor: false},

				comp.Offense{Damage: 10, Range: 10, TicksPerAttack: 10},
				comp.Reloader{AmmoCapacity: 10, AmmoQuantity: 0, ChamberCapacity: 10, ChamberQuantity: 0, NextReloadTick: 0, TicksPerReload: 30},
				comp.LastObservedTick{Tick: wCtx.CurrentTick()},
			)
			if err != nil {
				fmtErr := fmt.Errorf("error creating player: %w", err)
				SendError(wCtx, fmtErr.Error())
				return msg.CreatePlayerResult{}, fmtErr
			}

			SendSpawnPlayer(wCtx, id, create.Msg.TargetPersonaTag, spawnpoint)

			_ = physics.Instance().CreatePlayerBody(id, spawnpoint, world.Settings.PlayerRadius())

			return msg.CreatePlayerResult{Success: true}, nil
		})
}
