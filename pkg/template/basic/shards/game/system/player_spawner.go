package system

import (
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/component"
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/event"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type CreatePlayerCommand struct {
	Nickname string `json:"nickname"`
}

func (a CreatePlayerCommand) Name() string {
	return "create-player"
}

type CreatePlayerSystem struct{}

func (s *CreatePlayerSystem) Run(w *cardinal.World) {
	for cmd := range w.Commands[CreatePlayerCommand]() {
		command := cmd.Payload

		entity := w.Create[Player]()

		entity.Set(component.PlayerTag{Nickname: command.Nickname})
		entity.Set(component.Health{HP: 100})

		w.Broadcast(event.NewPlayer{Nickname: command.Nickname})
		w.Logger().Info().Uint32("entity", uint32(entity.ID())).Str("persona", cmd.Persona).
			Msgf("Created player %s", command.Nickname)
	}
}
