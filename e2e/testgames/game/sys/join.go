package sys

import (
	"fmt"

	"github.com/argus-labs/world-engine/example/tester/game/comp"
	"github.com/argus-labs/world-engine/example/tester/game/msg"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/types"
)

var PlayerEntityID = make(map[string]types.EntityID)

type Event struct {
	Message string `json:"message"`
}

func Join(ctx cardinal.WorldContext) error {
	logger := ctx.Logger()
	return cardinal.EachMessage[msg.JoinInput, msg.JoinOutput](
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
			err = ctx.EmitEvent(map[string]any{"message": fmt.Sprintf("%d player created", entityID)})
			if err != nil {
				return msg.JoinOutput{}, err
			}
			return msg.JoinOutput{Success: true}, nil
		},
	)
}
