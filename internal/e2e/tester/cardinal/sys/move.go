package sys

import (
	"fmt"
	"github.com/argus-labs/world-engine/example/tester/comp"
	"github.com/argus-labs/world-engine/example/tester/tx"
	"pkg.world.dev/world-engine/cardinal"
)

func Move(ctx cardinal.WorldContext) error {
	logger := ctx.Logger()
	tx.MoveTx.ForEach(ctx, func(mtx cardinal.TxData[tx.MoveInput]) (tx.MoveOutput, error) {
		logger.Info().Msgf("got move transaction from: %s", mtx.Sig().PersonaTag)
		playerEntityID, ok := PlayerEntityID[mtx.Sig().PersonaTag]
		if !ok {
			return tx.MoveOutput{}, fmt.Errorf("player %s has not joined yet", mtx.Sig().PersonaTag)
		}
		var resultingLoc comp.Location
		err := cardinal.UpdateComponent[comp.Location](ctx, playerEntityID, func(location *comp.Location) *comp.Location {
			switch mtx.Value().Direction {
			case "up":
				location.Y += 1
			case "down":
				location.Y -= 1
			case "left":
				location.X -= 1
			case "right":
				location.X += 1
			}
			resultingLoc = *location
			return location
		})
		if err != nil {
			return tx.MoveOutput{}, err
		}
		logger.Info().Msgf("player %s now at (%d, %d)", resultingLoc.X, resultingLoc.Y)
		return tx.MoveOutput{X: resultingLoc.X, Y: resultingLoc.Y}, err
	})
	return nil
}
