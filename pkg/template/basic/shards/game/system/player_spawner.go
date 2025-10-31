package system

import (
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/component"
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/event"

	"github.com/argus-labs/world-engine/pkg/cardinal"
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

		_, entity := state.Players.Create()

		entity.Tag.Set(component.PlayerTag{Nickname: command.Nickname})
		entity.Health.Set(component.Health{HP: 100})

		state.NewPlayerEvents.Emit(event.NewPlayer{Nickname: command.Nickname})
		state.Logger().Info().Uint32("entity", uint32(0)).Str("persona", cmd.Persona()).
			Msgf("Created player %s", command.Nickname)
	}
	return nil
}
