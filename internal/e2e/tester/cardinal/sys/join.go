package sys

import (
	"github.com/argus-labs/world-engine/example/tester/comp"
	"github.com/argus-labs/world-engine/example/tester/tx"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
)

var PlayerEntityID = make(map[string]entity.ID)

func Join(ctx cardinal.WorldContext) error {
	logger := ctx.Logger()
	tx.JoinTx.ForEach(ctx, func(jtx cardinal.TxData[tx.JoinInput]) (tx.JoinOutput, error) {
		logger.Info().Msgf("got join transaction from: %s", jtx.Sig().PersonaTag)
		entityID, err := cardinal.Create(ctx, comp.Location{}, comp.Player{})
		if err != nil {
			return tx.JoinOutput{}, err
		}
		err = cardinal.UpdateComponent[comp.Player](ctx, entityID, func(c *comp.Player) *comp.Player {
			c.ID = jtx.Sig().PersonaTag
			return c
		})
		if err != nil {
			return tx.JoinOutput{}, err
		}
		PlayerEntityID[jtx.Sig().PersonaTag] = entityID
		logger.Info().Msgf("player %s successfully joined", jtx.Sig().PersonaTag)
		return tx.JoinOutput{}, nil
	})
	return nil
}
