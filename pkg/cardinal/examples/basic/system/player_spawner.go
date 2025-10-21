package system

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/examples/basic/component"
	"github.com/argus-labs/world-engine/pkg/cardinal/examples/basic/event"
)

type CreatePlayerCommand struct {
	cardinal.BaseCommand
	Nickname string `json:"nickname"`
}

func (a CreatePlayerCommand) Name() string {
	return "create-player"
}

type CreatePlayerSystemState struct {
	cardinal.BaseSystemState
	CreatePlayerCommands cardinal.WithCommand[CreatePlayerCommand]
	NewPlayerEvents      cardinal.WithEvent[event.NewPlayer]
	Players              PlayerSearch
}

func CreatePlayerSystem(state *CreatePlayerSystemState) error {
	for cmd := range state.CreatePlayerCommands.Iter() {
		command := cmd.Payload()
		id, err := state.Players.Create(
			component.PlayerTag{Nickname: command.Nickname},
			component.Health{HP: 100},
		)
		if err != nil {
			// If we return the error, Cardinal will shutdown, so just log it.
			state.Logger().Error().Err(err).Msg("error creating entity")
			continue
		}

		state.NewPlayerEvents.Emit(event.NewPlayer{Nickname: command.Nickname})
		state.Logger().Info().Uint32("entity", uint32(id)).Str("persona", cmd.Persona()).
			Msgf("Created player %s", command.Nickname)
	}
	return nil
}
