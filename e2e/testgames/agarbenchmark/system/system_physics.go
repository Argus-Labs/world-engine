// nolint:mnd // agar shooter isn't linted for now. This will be a todo in the future.

package system

import (
	"errors"
	"fmt"
	"math"

	"github.com/ByteArena/box2d"
	comp "github.com/argus-labs/world-engine/example/tester/agarbenchmark/component"
	"github.com/argus-labs/world-engine/example/tester/agarbenchmark/msg"
	"github.com/argus-labs/world-engine/example/tester/agarbenchmark/physics"
	"github.com/argus-labs/world-engine/example/tester/agarbenchmark/world"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
)

func PhysicsSystem(wCtx cardinal.WorldContext) error {
	err := cardinal.EachMessage(
		wCtx,
		func(cmd message.TxData[msg.ChangeLinearVelocityMsg]) (msg.ChangeLinearVelocityResult, error) {
			entityID, err := queryLivingPlayer(wCtx, cmd.Tx.PersonaTag)
			if err != nil {
				if errors.Is(err, ErrPlayerDoesNotExist) {
					// The query didn't find anyone alive. That's fine.
					return msg.ChangeLinearVelocityResult{Success: true}, nil
				}
				// any other error should be returned.
				return msg.ChangeLinearVelocityResult{Success: false}, err
			}

			lvm := cmd.Msg
			lv := box2d.MakeB2Vec2(lvm.LinearVelocityX, lvm.LinearVelocityY)
			body, ok := physics.Instance().TryGetBody(entityID)
			if !ok {
				return msg.ChangeLinearVelocityResult{Success: false}, nil
			}
			body.SetLinearVelocity(lv)

			return msg.ChangeLinearVelocityResult{Success: true}, nil
		})
	if err != nil {
		DebugLogError(wCtx, "Failed to process linear velocity change:", err)
	}

	physics.Instance().Update()

	err = cardinal.NewSearch().
		Entity(
			filter.Contains(
				filter.Component[comp.Player](),
				filter.Component[comp.RigidBody](),
				filter.Component[comp.Position](),
				filter.Component[comp.LinearVelocity](),
				filter.Component[comp.Bearing]())).
		Each(wCtx, func(entityID types.EntityID) bool {
			rigidBody, err := cardinal.GetComponent[comp.RigidBody](wCtx, entityID)
			if err != nil {
				DebugLogError(wCtx, "Failed to get rigid body:", err)
				return true
			}
			if rigidBody.IsStatic || rigidBody.IsSensor {
				// Skip static and sensor entities. They're not moving.
				return true
			}

			body, ok := physics.Instance().TryGetBody(entityID)
			if !ok {
				wCtx.Logger().Warn().Msgf("PhysicsSystem: search for RigidBody failed: %d", entityID)
				return true
			}
			var personaTag string
			player, err := cardinal.GetComponent[comp.Player](wCtx, entityID)
			if err == nil {
				health, err := cardinal.GetComponent[comp.Health](wCtx, entityID)
				if err != nil {
					DebugLogError(wCtx, "Failed to get health:", err)
					return true
				}
				// Skip dead players. They're not moving.
				if !health.IsAlive() {
					return true
				}
				personaTag = player.PersonaTag
			} else {
				personaTag = "pickup" // Unused but mildly informative for debugging on the client side.
			}

			err = errors.Join(cardinal.UpdateComponent(wCtx, entityID, func(pos *comp.Position) *comp.Position {
				if (math.Abs(pos.X-body.GetPosition().X) < 0.0001) && (math.Abs(pos.Y-body.GetPosition().Y) < 0.0001) {
					return pos
				}
				p := body.GetPosition()
				pos.X = p.X
				pos.Y = p.Y
				SendPosition(wCtx, entityID, personaTag, p)
				return pos
			}), cardinal.UpdateComponent(wCtx, entityID, func(rot *comp.Bearing) *comp.Bearing {
				lv := body.GetLinearVelocity()
				if lv.LengthSquared() < 0.0001 {
					return rot
				}
				degrees := math.Atan2(lv.Y, lv.X) * (180.0 / math.Pi)
				if (math.Abs(rot.Degrees - degrees)) < 0.0001 {
					return rot
				}
				rot.Degrees = degrees
				SendBearing(wCtx, entityID, personaTag, rot.Degrees)
				return rot
			}), cardinal.UpdateComponent(wCtx, entityID, func(vel *comp.LinearVelocity) *comp.LinearVelocity {
				lv := body.GetLinearVelocity()
				if (math.Abs(vel.X-lv.X) < 0.0001) && (math.Abs(vel.Y-lv.Y) < 0.0001) {
					return vel
				}
				vel.X = lv.X
				vel.Y = lv.Y
				SendLinearVelocity(wCtx, entityID, personaTag, lv)
				return vel
			}))
			if err != nil {
				DebugLogError(wCtx, "Failed to update position, bearing, or linear velocity:", err)
				return true
			}

			return true
		})
	if err != nil {
		return fmt.Errorf("PhysicsSystem: search for RigidBody failed: %w", err)
	}

	// We're emitting contacts last in order to give the client a chance to sync with entity info first.
	for {
		select {
		case pair := <-physics.Instance().Contacts:
			wCtx.Logger().Debug().Msgf("Handling contact pair: %d -> %d", pair.EntityID0, pair.EntityID1)
			entity0, err := cardinal.GetComponent[comp.RigidBody](wCtx, pair.EntityID0)
			if err != nil {
				DebugLogError(wCtx, "entity0 comp.RigidBody issue:", err)
				continue
			}
			entity1, err := cardinal.GetComponent[comp.RigidBody](wCtx, pair.EntityID1)
			if err != nil {
				DebugLogError(wCtx, "entity1 comp.RigidBody issue:", err)
				continue
			}

			a := pair.EntityID0
			b := pair.EntityID1

			// Ensure that a is the sensor and b is the player.
			if entity1.IsSensor {
				a, b = b, a
			}

			if entity0.IsSensor || entity1.IsSensor {
				wCtx.Logger().Debug().Msg("One of these is a sensor. That's nice.")
				pickup, err := cardinal.GetComponent[comp.Pickup](wCtx, a)
				if err != nil {
					DebugLogError(wCtx, "pickup comp.Pickup issue:", err)
					continue
				}
				switch pickup.ID {
				case world.CoinPickup:
					wCtx.Logger().Debug().Msg("Coin pickup sensor triggered")
					err = cardinal.UpdateComponent(wCtx, b, func(wealth *comp.Wealth) *comp.Wealth {
						wealth.Value++
						return wealth
					})
					if err != nil {
						DebugLogError(wCtx, "Failed to update wealth:", err)
					}
					err = cardinal.UpdateComponent(wCtx, b, func(score *comp.Score) *comp.Score {
						score.Value += world.Settings.CoinScore()
						SendCoinPickup(wCtx, a, b, score.Value)
						return score
					})
					if err != nil {
						DebugLogError(wCtx, "Failed to update score:", err)
					}
					wCtx.Logger().Info().Msg("Removing coin pickup.")
					//RemovePhysicalEntity(wCtx, a)
					SchedulePhysicalEntityForLateRemoval(a)
				case world.MedpackPickup:
					wCtx.Logger().Debug().Msg("Medpack pickup sensor triggered.")
					err = cardinal.UpdateComponent(wCtx, b, func(score *comp.Score) *comp.Score {
						score.Value += world.Settings.CoinScore()
						return score
					})
					if err != nil {
						DebugLogError(wCtx, "MedpackPickup: Failed to update score.", err)
					} else {
						err = cardinal.UpdateComponent(wCtx, b, func(health *comp.Health) *comp.Health {
							health.Value = min(world.Settings.MaxPlayerHealth(), health.Value+world.Settings.MedpackScore())
							return health
						})
						if err != nil {
							DebugLogError(wCtx, "MedpackPickup: Failed to update health.", err)
						} else {
							health, err := cardinal.GetComponent[comp.Health](wCtx, b)
							if err != nil {
								DebugLogError(wCtx, "MedpackPickup: Failed to get health.", err)
							}
							SendMedpackPickup(wCtx, a, b, health.Value)
							wCtx.Logger().Info().Msg("Removing medpack pickup.")
							//RemovePhysicalEntity(wCtx, a)
							SchedulePhysicalEntityForLateRemoval(a)
						}
					}
				}
			}
			wCtx.Logger().Debug().Msgf("Contact pair handled? %d -> %d", pair.EntityID0, pair.EntityID1)
		default:
			return nil
		}
	}
}
