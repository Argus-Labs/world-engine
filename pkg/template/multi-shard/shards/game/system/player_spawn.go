package system

import (
	"fmt"
	"time"

	otherworld "github.com/argus-labs/world-engine/pkg/template/multi-shard/pkg/other_world"
	chatcommand "github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/command"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/command"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/component"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/event"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type SpawnPlayerSystemState struct {
	cardinal.BaseSystemState
	SpawnPlayerCommands cardinal.WithCommand[command.PlayerSpawn]
	PlayerSpawnEvent    cardinal.WithEvent[event.PlayerSpawn]
	Players             PlayerSearch
}

func PlayerSpawnSystem(state *SpawnPlayerSystemState) error {
	for cmd := range state.SpawnPlayerCommands.Iter() {
		command := cmd.Payload()

		// Regardless of whether the player exists or not, we emit a spawn event
		// Because the act of spawning is also creating (if they don’t already exist)
		state.PlayerSpawnEvent.Emit(event.PlayerSpawn{
			ArgusAuthID:   command.ArgusAuthID,
			ArgusAuthName: command.ArgusAuthName,
			X:             command.X,
			Y:             command.Y,
		})

		if playerSet.Exists(command.ArgusAuthID) {
			state.Logger().Info().Msgf("Player with ID %s already exists, skipping creation", command.ArgusAuthID)
			continue
		}

		id, player := state.Players.Create()
		player.Tag.Set(component.PlayerTag{ArgusAuthID: command.ArgusAuthID, ArgusAuthName: command.ArgusAuthName})
		player.Position.Set(component.Position{X: int(command.X), Y: int(command.Y)})
		player.Online.Set(component.OnlineStatus{Online: true, LastActive: time.Now()})

		playerSet.Add(command.ArgusAuthID)

		state.Logger().Info().
			Uint32("entity", uint32(id)).
			Msgf("Created player %s (id: %s)", command.ArgusAuthName, command.ArgusAuthID)

		// Inform chat shard about the spawn
		otherworld.Chat.Send(&state.BaseSystemState, chatcommand.UserChat{
			ArgusAuthID:   command.ArgusAuthID,
			ArgusAuthName: command.ArgusAuthName,
			Message:       fmt.Sprintf("%s joined at (%s)", command.ArgusAuthName, state.Timestamp().Format(time.RFC3339)),
		})
	}
	return nil
}
