package system

import (
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/command"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/component"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/event"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type PlayerLeaveSystemState struct {
	cardinal.BaseSystemState
	PlayerLeaveCommands  cardinal.WithCommand[command.PlayerLeave]
	PlayerDepartureEvent cardinal.WithEvent[event.PlayerDeparture]
}

// PlayerLeaveSystem is called when a player leaves a quadrant (e.g. to join another quadrant).
func PlayerLeaveSystem(state *PlayerLeaveSystemState) {
	players := make(map[string]cardinal.Entity)

	for player := range state.Exact[Player]().Iter() {
		players[player.Get[component.PlayerTag]().ArgusAuthID] = player
	}

	for cmd := range state.PlayerLeaveCommands.Iter() {
		command := cmd.Payload

		entity, exists := players[command.ArgusAuthID]
		if !exists {
			state.Logger().Info().Msgf("Player with ID %s not found", command.ArgusAuthID)
			continue
		}

		entity.Destroy()

		state.PlayerDepartureEvent.Broadcast(event.PlayerDeparture{
			ArgusAuthID: command.ArgusAuthID,
		})
	}
}
