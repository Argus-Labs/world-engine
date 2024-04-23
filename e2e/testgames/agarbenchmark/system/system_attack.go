package system

import (
	"fmt"
	"math"

	"github.com/ByteArena/box2d"
	comp "github.com/argus-labs/world-engine/example/tester/agarbenchmark/component"
	"github.com/argus-labs/world-engine/example/tester/agarbenchmark/physics"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
)

// Might not need all these components.
func AttackSystem(wCtx cardinal.WorldContext) error {
	err := cardinal.NewSearch().
		Entity(filter.Contains(filter.Component[comp.Player]())).
		Each(wCtx, func(entityID types.EntityID) bool {

			reloader, err := cardinal.GetComponent[comp.Reloader](wCtx, entityID)
			if err != nil {
				DebugLogError(wCtx, "AttackSystem: Error getting reloader component", err)
				return true
			}

			// Never nester hell!
			if reloader.ChamberQuantity == 0 && reloader.AmmoCapacity > 0 {
				if reloader.AmmoQuantity > 0 {
					numRoundsToReload := min(reloader.AmmoQuantity, reloader.ChamberCapacity)
					if numRoundsToReload > 0 {
						if reloader.NextReloadTick < wCtx.CurrentTick() {
							reloader.NextReloadTick = wCtx.CurrentTick() + reloader.TicksPerReload
							if reloader.NextReloadTick <= wCtx.CurrentTick() {
								reloader.ChamberQuantity = numRoundsToReload
								reloader.AmmoQuantity -= numRoundsToReload
							}
						}
					}
				}
			}
			err = cardinal.SetComponent(wCtx, entityID, reloader)
			if err != nil {
				DebugLogError(wCtx, "AttackSystem: Error setting reloader component", err)
				return true
			}

			waitForNextAttack := func(wCtx cardinal.WorldContext, attackerEntityID types.EntityID) {
				offense, err := cardinal.GetComponent[comp.Offense](wCtx, attackerEntityID)
				if err != nil {
					DebugLogError(wCtx, "AttackSystem: Error getting offense component", err)
					return
				}
				offense.NextAttackTick = wCtx.CurrentTick() + offense.TicksPerAttack
				err = cardinal.SetComponent(wCtx, attackerEntityID, offense)
				if err != nil {
					DebugLogError(wCtx, "AttackSystem: Error setting offense component", err)
				}
			}

			// Attack function
			attack := func(wCtx cardinal.WorldContext, attackerEntityID types.EntityID, targetEntityID types.EntityID, damage int) error {
				// We are facing the target. Attack targetHealth!
				targetHealth, err := cardinal.GetComponent[comp.Health](wCtx, targetEntityID)
				if err != nil {
					DebugLogError(wCtx, "AttackSystem: Error getting health component", err)
					return err
				}
				targetHealth.Value = max(0, targetHealth.Value-damage)
				if targetHealth.IsAlive() {
					SendHit(wCtx, attackerEntityID, targetEntityID, damage, targetHealth.Value)
				} else {
					SendKill(wCtx, attackerEntityID, targetEntityID)
					wCtx.Logger().Info().Msgf("Removing player: %d", targetEntityID)
					SchedulePhysicalEntityForLateRemoval(targetEntityID)
				}
				err = cardinal.SetComponent(wCtx, targetEntityID, targetHealth)
				if err != nil {
					DebugLogError(wCtx, "AttackSystem: Error setting health component", err)
					return err
				}
				return nil
			}

			offense, err := cardinal.GetComponent[comp.Offense](wCtx, entityID)
			if err != nil {
				DebugLogError(wCtx, "AttackSystem: Error getting offense component", err)
				return true
			}
			if offense.NextAttackTick > wCtx.CurrentTick() {
				return true // not yet ready to shoot
			}
			position, err := cardinal.GetComponent[comp.Position](wCtx, entityID)
			if err != nil {
				DebugLogError(wCtx, "AttackSystem: Error getting position component", err)
				return true
			}
			attackerPosition := box2d.MakeB2Vec2(position.X, position.Y)

			// Find the closest opponent to attack.
			targetEntityID, wasFound := physics.Instance().FindClosestNeighbor(attackerPosition, offense.Range, func(hitEntityID types.EntityID) bool {
				if hitEntityID == entityID {
					return false // skip
				}
				// Could maybe just attack anything with health? Assuming no for now.
				_, err := cardinal.GetComponent[comp.Player](wCtx, hitEntityID)
				if err != nil {
					return false // skip
				}
				opponentHealth, _ := cardinal.GetComponent[comp.Health](wCtx, hitEntityID)
				return opponentHealth.IsAlive() // Consider alive opponents only.
			})
			if !wasFound {
				return true
			}

			targetPosition, err := cardinal.GetComponent[comp.Position](wCtx, targetEntityID)
			if err != nil {
				DebugLogError(wCtx, "AttackSystem: Error getting target position component", err)
				return true
			}

			tp := box2d.MakeB2Vec2(targetPosition.X, targetPosition.Y)
			d := box2d.B2Vec2Sub(tp, attackerPosition)
			_ = d.Normalize()

			bearing, err := cardinal.GetComponent[comp.Bearing](wCtx, targetEntityID)
			if err != nil {
				DebugLogError(wCtx, "AttackSystem: Error getting bearing component", err)
				return true
			}
			angle := bearing.Degrees * math.Pi / 180.0
			bearingVector := box2d.B2Vec2{X: math.Cos(angle), Y: math.Sin(angle)}
			dp := box2d.B2Vec2Dot(bearingVector, d)

			if dp < 0.0 {
				// We are facing the target. Attack health!
				attackErr := attack(wCtx, entityID, targetEntityID, offense.Damage)
				if attackErr != nil {
					fmt.Println("AttackSystem: failed to attack: ", attackErr.Error())
					return true
				}
			} else {
				// We are behind the target. Attack their coinpack!
				targetWealth, err := cardinal.GetComponent[comp.Wealth](wCtx, targetEntityID)
				if err != nil {
					DebugLogError(wCtx, "AttackSystem: Error getting wealth component", err)
					return true
				}
				if targetWealth.Value > 0 {
					targetWealth.Value = max(0, targetWealth.Value-1) // offense.Damage)
					SendHitCoinpack(wCtx, entityID, targetEntityID, targetWealth.Value)
					setErr := cardinal.SetComponent(wCtx, targetEntityID, targetWealth)
					if setErr != nil {
						fmt.Println("AttackSystem: failed to set targetWealth component: ", setErr.Error())
						return true
					}
				} else { // target coin is empty, deal damage to health instead
					attackErr := attack(wCtx, entityID, targetEntityID, offense.Damage)
					if attackErr != nil {
						fmt.Println("AttackSystem: failed to attack: ", attackErr.Error())
						return true
					}
				}
			}
			// This is intended for all types of attack: health & wealth
			waitForNextAttack(wCtx, entityID)
			return true
		})
	if err != nil {
		return fmt.Errorf("AttackSystem: failed to search: %w", err)
	}
	return nil
}
