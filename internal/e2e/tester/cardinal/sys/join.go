package sys

import (
	"fmt"
	"github.com/argus-labs/world-engine/example/tester/msg"

	"github.com/argus-labs/world-engine/example/tester/comp"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
)

var PlayerEntityID = make(map[string]entity.ID)

func Join(ctx cardinal.WorldContext) error {
	logger := ctx.Logger()
	msg.JoinMsg.ForEach(ctx, func(jtx cardinal.TxData[msg.JoinInput]) (msg.JoinOutput, error) {
		logger.Info().Msgf("got join transaction from: %s", jtx.Tx().PersonaTag)
		entityID, err := cardinal.Create(ctx, comp.Location{}, comp.Player{})
		if err != nil {
			return msg.JoinOutput{}, err
		}
		err = cardinal.UpdateComponent[comp.Player](ctx, entityID, func(c *comp.Player) *comp.Player {
			c.ID = jtx.Tx().PersonaTag
			return c
		})
		if err != nil {
			return msg.JoinOutput{}, err
		}
		PlayerEntityID[jtx.Tx().PersonaTag] = entityID
		logger.Info().Msgf("player %s successfully joined", jtx.Tx().PersonaTag)
		ctx.EmitEvent(fmt.Sprintf("%d player created", entityID))
		return msg.JoinOutput{}, nil
	})
	return nil
}
