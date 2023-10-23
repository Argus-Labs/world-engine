package sys

import (
	"github.com/argus-labs/world-engine/example/tester/comp"
	"github.com/argus-labs/world-engine/example/tester/tx"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
)

var PlayerEntityID = make(map[string]entity.ID)

func Join(ctx cardinal.WorldContext) error {
	logger := ctx.Logger()
	for _, jtx := range tx.JoinTx.In(ctx) {
		logger.Info().Msgf("got join transaction from: %s", jtx.Sig().PersonaTag)
		entity, err := cardinal.Create(ctx, comp.Location{}, comp.Player{})
		if err != nil {

			tx.JoinTx.AddError(, jtx.Hash(), err)
			continue
		}
		component.UpdateComponent[comp.Player](ctx, entity, func(c *comp.Player) *comp.Player {
			c.ID = jtx.Sig.PersonaTag
			return c
		})
		if err != nil {
			tx.JoinTx.AddError(ctx, jtx.TxHash, err)
			continue
		}
		PlayerEntityID[jtx.Sig.PersonaTag] = entity
	}
	return nil
}
