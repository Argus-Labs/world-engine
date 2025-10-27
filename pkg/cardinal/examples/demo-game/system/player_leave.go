package system

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/examples/demo-game/event"
)

type PlayerLeaveCommand struct {
	cardinal.BaseCommand
	ArgusAuthID string `json:"argus_auth_id"`
}

func (p PlayerLeaveCommand) Name() string {
	return "player-leave"
}

type PlayerLeaveSystemState struct {
	cardinal.BaseSystemState
	PlayerLeaveCommands  cardinal.WithCommand[PlayerLeaveCommand]
	PlayerDepartureEvent cardinal.WithEvent[event.PlayerDeparture]
	Players              PlayerSearch
}

// PlayerLeaveSystem is called when a player leaves a quadrant (e.g. to join another quadrant).
func PlayerLeaveSystem(state *PlayerLeaveSystemState) error {
	players := make(map[string]ecs.EntityID)

	for entity, player := range state.Players.Iter() {
		players[player.Tag.Get().ArgusAuthID] = entity
	}

	for cmd := range state.PlayerLeaveCommands.Iter() {
		command := cmd.Payload()

		entityID, exists := players[command.ArgusAuthID]
		if !exists {
			state.Logger().Info().Msgf("Player with ID %s not found", command.ArgusAuthID)
			continue
		}

		state.Players.Destroy(entityID)

		state.PlayerDepartureEvent.Emit(event.PlayerDeparture{
			ArgusAuthID: command.ArgusAuthID,
		})
	}
	return nil
}
