package system

import (
	"time"

	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/command"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/component"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/event"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type MovePlayerSystemState struct {
	cardinal.BaseSystemState
	MovePlayerCommands  cardinal.WithCommand[command.MovePlayer]
	PlayerSpawnEvent    cardinal.WithEvent[event.PlayerSpawn]
	PlayerMovementEvent cardinal.WithEvent[event.PlayerMovement]
	Players             PlayerSearch
}

func MovePlayerSystem(state *MovePlayerSystemState) error {
	for cmd := range state.MovePlayerCommands.Iter() {
		command := cmd.Payload()

		for entity, player := range state.Players.Iter() {
			tag := player.Tag.Get()

			if command.ArgusAuthID != tag.ArgusAuthID {
				continue
			}

			isOnline := player.Online.Get().Online

			if !isOnline {
				state.PlayerSpawnEvent.Emit(event.PlayerSpawn{
					ArgusAuthID:   tag.ArgusAuthID,
					ArgusAuthName: tag.ArgusAuthName,
					X:             command.X,
					Y:             command.Y,
				})
			}

			player.Position.Set(component.Position{X: int(command.X), Y: int(command.Y)})
			player.Online.Set(component.OnlineStatus{Online: true, LastActive: time.Now()})

			state.PlayerMovementEvent.Emit(event.PlayerMovement{
				ArgusAuthID: tag.ArgusAuthID,
				X:           command.X,
				Y:           command.Y,
			})

			name := tag.ArgusAuthName

			state.Logger().Info().
				Uint32("entity", uint32(entity)).
				Msgf("Player %s (id: %s) moved to %d, %d", name, tag.ArgusAuthID, command.X, command.Y)
		}
	}
	return nil
}
