package sys

import (
	"fmt"

	"github.com/argus-labs/world-engine/example/tester/game/comp"
	"github.com/argus-labs/world-engine/example/tester/game/msg"

	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/world"
)

var PlayerEntityID = make(map[string]types.EntityID)

func Join(wCtx world.WorldContext) error {
	logger := wCtx.Logger()
	return world.EachMessage[msg.JoinInput](
		wCtx, func(jtx world.Tx[msg.JoinInput]) (any, error) {
			logger.Info().Msgf("got join transaction from: %s", jtx.Tx.PersonaTag)
			entityID, err := world.Create(wCtx, comp.Location{}, comp.Player{})
			if err != nil {
				return nil, err
			}
			err = world.UpdateComponent[comp.Player](
				wCtx, entityID, func(c *comp.Player) *comp.Player {
					c.ID = jtx.Tx.PersonaTag
					return c
				},
			)
			if err != nil {
				return nil, err
			}
			PlayerEntityID[jtx.Tx.PersonaTag] = entityID
			logger.Info().Msgf("player %s successfully joined", jtx.Tx.PersonaTag)

			wCtx.EmitEvent(map[string]any{"message": fmt.Sprintf("%d player created", entityID)})
			wCtx.EmitStringEvent("this is a string event")

			return nil, nil
		},
	)
}
