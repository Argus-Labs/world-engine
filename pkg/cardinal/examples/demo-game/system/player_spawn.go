package system

import (
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/examples/demo-game/component"
	"github.com/argus-labs/world-engine/pkg/cardinal/examples/demo-game/event"
)

type PlayerSpawnCommand struct {
	cardinal.BaseCommand
	ArgusAuthID   string `json:"argus_auth_id"`
	ArgusAuthName string `json:"argus_auth_name"`
	X             uint32 `json:"x"`
	Y             uint32 `json:"y"`
}

func (a PlayerSpawnCommand) Name() string {
	return "player-spawn"
}

type SpawnPlayerSystemState struct {
	cardinal.BaseSystemState
	SpawnPlayerCommands cardinal.WithCommand[PlayerSpawnCommand]
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
	}
	return nil
}
