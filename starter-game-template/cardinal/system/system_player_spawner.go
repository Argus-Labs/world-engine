package system

import (
	"fmt"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/message"

	comp "github.com/argus-labs/starter-game-template/cardinal/component"
	"github.com/argus-labs/starter-game-template/cardinal/msg"
)

// PlayerSpawnerSystem spawns players based on `CreatePlayer` transactions.
// This provides an example of a system that creates a new entity.
func PlayerSpawnerSystem(world cardinal.WorldContext) error {
	return cardinal.EachMessage[msg.CreatePlayerMsg, msg.CreatePlayerResult](
		world,
		func(create message.TxData[msg.CreatePlayerMsg]) (msg.CreatePlayerResult, error) {
			maxHp := 100
			id, err := cardinal.Create(world,
				comp.Player{Nickname: create.Msg.Nickname},
				comp.Health{HP: maxHp},
			)
			if err != nil {
				return msg.CreatePlayerResult{}, fmt.Errorf("error creating player: %w", err)
			}

			err = world.EmitEvent(fmt.Sprintf("new player %d created", id))
			if err != nil {
				return msg.CreatePlayerResult{}, err
			}
			return msg.CreatePlayerResult{Success: true}, nil
		})
	return nil
}
