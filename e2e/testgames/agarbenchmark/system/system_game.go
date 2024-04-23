package system

import (
	"errors"
	"fmt"
	"math/rand"

	"github.com/ByteArena/box2d"
	comp "github.com/argus-labs/world-engine/example/tester/agarbenchmark/component"
	"github.com/argus-labs/world-engine/example/tester/agarbenchmark/physics"
	"github.com/argus-labs/world-engine/example/tester/agarbenchmark/world"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

type GameState int

var ErrPlayerDoesNotExist = errors.New("player does not exist")

const InitializationState GameState = 0
const LoadingState GameState = 1
const GameplayState GameState = 2

var State = InitializationState
var rng *rand.Rand

func GameSystem(wCtx cardinal.WorldContext) error {
	if State == InitializationState {
		seed := world.Settings.Seed()
		rng = rand.New(rand.NewSource(int64(seed)))
		State = LoadingState
	} else if State == LoadingState {
		if wCtx.CurrentTick() > 50 {
			State = GameplayState
		}
	}
	return nil
}

func DebugLogError(wCtx cardinal.WorldContext, message string, err error) {
	fmtErr := fmt.Sprintln(message, err.Error())
	SendError(wCtx, fmtErr)
	wCtx.Logger().Error().Msgf(fmtErr)
}

func SpawnCoin(wCtx cardinal.WorldContext, p box2d.B2Vec2) types.EntityID {
	entityID := spawnPickup(wCtx, world.CoinPickup, p, world.Settings.CoinRadius(), comp.Score{Value: world.Settings.CoinScore()}, comp.Coin{})
	if State == GameplayState {
		SendSpawnCoin(wCtx, entityID, p)
	}
	return entityID
}

func SpawnMedpack(wCtx cardinal.WorldContext, p box2d.B2Vec2) types.EntityID {
	entityID := spawnPickup(wCtx, world.MedpackPickup, p, world.Settings.MedpackRadius(), comp.Score{Value: world.Settings.MedpackScore()}, comp.Medpack{})
	if State == GameplayState {
		SendSpawnMedpack(wCtx, entityID, p)
	}
	return entityID
}

// Note: Once a float64 is tumbled around through a few math calculations,
// the outcome may not be reproduceable on different platforms or with different versions of Go.
// We need to know if this is ok.
func getRandomGridPosition() box2d.B2Vec2 {
	gridWidth := world.Settings.GridWidth()
	gridHeight := world.Settings.GridHeight()
	return box2d.B2Vec2{
		X: (rng.Float64() - .5) * float64(gridWidth),
		Y: (rng.Float64() - .5) * float64(gridHeight),
	}
}

// TODO: Ensure this pickup is not spawned on top of another pickup.
func spawnPickup(wCtx engine.Context, pickupID world.PickupType, position box2d.B2Vec2, radius float64, components ...types.Component) types.EntityID {
	components = append(
		components,
		comp.Pickup{ID: pickupID},
		comp.Position{X: position.X, Y: position.Y},
		comp.Radius{Length: radius},
		comp.RigidBody{IsStatic: true, IsSensor: true})
	entityID, err := cardinal.Create(
		wCtx,
		components...)
	if err != nil {
		panic(err)
	}
	_ = physics.Instance().CreatePickupBody(entityID, position, radius)
	return entityID
}

// queryTargetPlayer queries for the target player's entity ID, health, and actor components.
func queryLivingPlayer(world cardinal.WorldContext, targetPersonaTag string) (types.EntityID, error) {
	var wasFound bool
	var wasFoundAlive bool
	var playerID types.EntityID
	err := cardinal.NewSearch().
		Entity(filter.Contains(filter.Component[comp.Player](), filter.Component[comp.Health]())).
		Each(world,
			func(id types.EntityID) bool {
				player, err := cardinal.GetComponent[comp.Player](world, id)
				if err != nil {
					DebugLogError(world, "Failed to get player:", err)
					return true
				}

				// Terminates the search if the player is found
				if player.PersonaTag == targetPersonaTag {
					wasFound = true
					health, err := cardinal.GetComponent[comp.Health](world, id)
					if err != nil {
						DebugLogError(world, "Failed to get health:", err)
						return true
					}
					if health.IsAlive() {
						playerID = id
						wasFoundAlive = true
						return false
					}
				}

				// Continue searching if the player is not the target player
				return true
			})
	if err != nil {
		return 0, err
	}
	if !wasFound {
		return 0, fmt.Errorf("%w: %q", ErrPlayerDoesNotExist, targetPersonaTag)
	} else if !wasFoundAlive {
		return 0, fmt.Errorf("%w: player %q is no longer alive", ErrPlayerDoesNotExist, targetPersonaTag)
	}

	return playerID, nil
}
