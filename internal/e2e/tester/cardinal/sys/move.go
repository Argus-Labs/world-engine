package sys

import (
	"fmt"
	"github.com/argus-labs/world-engine/example/tester/comp"
	"github.com/argus-labs/world-engine/example/tester/tx"
	"pkg.world.dev/world-engine/cardinal"
)

func Move(ctx cardinal.WorldContext) error {
	logger := ctx.Logger()
	for _, mtx := range tx.MoveTx.In(ctx) {
		logger.Info().Msgf("got move transaction from: %s", mtx.Sig().PersonaTag)
		playerEntityID, ok := PlayerEntityID[mtx.Sig().PersonaTag]
		if !ok {
			tx.MoveTx.AddError(ctx, mtx.TxHash, fmt.Errorf("player %s has not joined yet", mtx.Sig.PersonaTag))
		}
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
			return location
		})
		if err != nil {
			tx.MoveTx.AddError(ctx, mtx.TxHash, err)
		}
	}
	return nil
}
