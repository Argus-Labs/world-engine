package sys

import (
	"fmt"

	"github.com/argus-labs/world-engine/example/tester/game/comp"
	"github.com/argus-labs/world-engine/example/tester/game/msg"

	"pkg.world.dev/world-engine/cardinal/world"
)

func Move(ctx world.WorldContext) error {
	logger := ctx.Logger()
	return world.EachMessage[msg.MoveInput](
		ctx, func(mtx world.Tx[msg.MoveInput]) (any, error) {
			logger.Info().Msgf("got move transaction from: %s", mtx.Tx.PersonaTag)
			playerEntityID, ok := PlayerEntityID[mtx.Tx.PersonaTag]
			if !ok {
				return nil, fmt.Errorf("player %s has not joined yet", mtx.Tx.PersonaTag)
			}
			var resultingLoc comp.Location
			err := world.UpdateComponent[comp.Location](
				ctx, playerEntityID,
				func(location *comp.Location) *comp.Location {
					switch mtx.Msg.Direction {
					case "up":
						location.Y++
					case "down":
						location.Y--
					case "left":
						location.X--
					case "right":
						location.X++
					}
					resultingLoc = *location
					return location
				})
			if err != nil {
				return nil, err
			}
			logger.Info().Msgf("player %s now at (%d, %d)", mtx.Tx.PersonaTag, resultingLoc.X, resultingLoc.Y)
			return nil, nil
		})
}
