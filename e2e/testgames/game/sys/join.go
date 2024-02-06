package sys

import (
	"fmt"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"

	"github.com/argus-labs/world-engine/example/tester/game/comp"
	"github.com/argus-labs/world-engine/example/tester/game/msg"
	"pkg.world.dev/world-engine/cardinal"
)

var PlayerEntityID = make(map[string]types.EntityID)

func Join(ctx engine.Context) error {
	logger := ctx.Logger()
	msg.JoinMsg.Each(
		ctx, func(jtx message.TxData[msg.JoinInput]) (msg.JoinOutput, error) {
			logger.Info().Msgf("got join transaction from: %s", jtx.Tx.PersonaTag)
			entityID, err := cardinal.Create(ctx, comp.Location{}, comp.Player{})
			if err != nil {
				return msg.JoinOutput{}, err
			}
			err = cardinal.UpdateComponent[comp.Player](
				ctx, entityID, func(c *comp.Player) *comp.Player {
					c.ID = jtx.Tx.PersonaTag
					return c
				},
			)
			if err != nil {
				return msg.JoinOutput{}, err
			}
			PlayerEntityID[jtx.Tx.PersonaTag] = entityID
			logger.Info().Msgf("player %s successfully joined", jtx.Tx.PersonaTag)
			ctx.EmitEvent(fmt.Sprintf("%d player created", entityID))
			return msg.JoinOutput{Success: true}, nil
		},
	)
	return nil
}
