package sys

import (
	"fmt"
	"github.com/argus-labs/world-engine/example/tester/comp"
	"github.com/argus-labs/world-engine/example/tester/msg"
	"pkg.world.dev/world-engine/cardinal"
)

func Move(ctx cardinal.WorldContext) error {
	logger := ctx.Logger()
	msg.MoveMsg.ForEach(ctx, func(mtx cardinal.TxData[msg.MoveInput]) (msg.MoveOutput, error) {
		logger.Info().Msgf("got move transaction from: %s", mtx.Tx().PersonaTag)
		playerEntityID, ok := PlayerEntityID[mtx.Tx().PersonaTag]
		if !ok {
			return msg.MoveOutput{}, fmt.Errorf("player %s has not joined yet", mtx.Tx().PersonaTag)
		}
		var resultingLoc comp.Location
		err := cardinal.UpdateComponent[comp.Location](ctx, playerEntityID, func(location *comp.Location) *comp.Location {
			switch mtx.Msg().Direction {
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
			return msg.MoveOutput{}, err
		}
		logger.Info().Msgf("player %s now at (%d, %d)", mtx.Tx().PersonaTag, resultingLoc.X, resultingLoc.Y)
		return msg.MoveOutput{X: resultingLoc.X, Y: resultingLoc.Y}, err
	})
	return nil
}
