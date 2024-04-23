package system

import (
	"errors"

	comp "github.com/argus-labs/world-engine/example/tester/agarbenchmark/component"
	"github.com/argus-labs/world-engine/example/tester/agarbenchmark/msg"
	"github.com/argus-labs/world-engine/example/tester/agarbenchmark/world"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
)

func KeepAliveSystem(wCtx cardinal.WorldContext) error {
	err := cardinal.EachMessage(
		wCtx,
		func(keepAlive message.TxData[msg.KeepAliveMsg]) (msg.KeepAliveResult, error) {
			entityID, err := queryLivingPlayer(wCtx, keepAlive.Tx.PersonaTag)
			if err != nil {
				if errors.Is(err, ErrPlayerDoesNotExist) {
					// The query didn't find anyone alive. That's fine.
					return msg.KeepAliveResult{Success: true}, nil
				}
				// any other error should be returned.
				return msg.KeepAliveResult{Success: false}, err
			}
			err = cardinal.UpdateComponent(wCtx, entityID, func(lastObservedTick *comp.LastObservedTick) *comp.LastObservedTick {
				lastObservedTick.Tick = wCtx.CurrentTick()
				return lastObservedTick
			})
			if err != nil {
				DebugLogError(wCtx, "KeepAliveSystem: Failed to update last observed tick:", err)
				return msg.KeepAliveResult{Success: false}, err
			}
			return msg.KeepAliveResult{Success: true}, nil
		})
	if err != nil {
		DebugLogError(wCtx, "KeepAliveSystem: Failed to process keep alive:", err)
		return err
	}

	err = cardinal.NewSearch().
		Entity(filter.Contains(filter.Component[comp.Player]())).
		Each(wCtx, func(entityID types.EntityID) bool {
			lastObservedTick, err := cardinal.GetComponent[comp.LastObservedTick](wCtx, entityID)
			if err != nil {
				DebugLogError(wCtx, "Failed to get lastObservedTick:", err)
				return true
			}
			maxDelta := uint64(world.Settings.KeepAliveInterval())
			if wCtx.CurrentTick()-lastObservedTick.Tick > maxDelta {
				wCtx.Logger().Info().Msgf("Player %d timed out.", entityID)
				SchedulePhysicalEntityForLateRemoval(entityID)
				SendCull(wCtx, entityID)
			}
			return true
		})
	if err != nil {
		DebugLogError(wCtx, "KeepAliveSystem: Failed to process keep alive:", err)
		return err
	}

	return nil
}
