package system

import (
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/component"
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/event"
	systemevent "github.com/argus-labs/world-engine/pkg/template/basic/shards/game/system_event"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type AttackPlayerCommand struct {
	Target string
	Damage uint32
}

func (a AttackPlayerCommand) Name() string {
	return "attack-player"
}

type AttackPlayerSystem struct{}

func (s *AttackPlayerSystem) Run(w *cardinal.World) {
	players := w.Exact[Player]()
	for cmd := range w.Commands[AttackPlayerCommand]() {
		command := cmd.Payload
		for player := range players.Iter() {
			entity := player.ID()
			tag := player.Get[component.PlayerTag]()

			if command.Target != tag.Nickname {
				continue
			}

			newHealth := player.Get[component.Health]().HP - int(command.Damage)
			if newHealth > 0 {
				player.Set(component.Health{HP: newHealth})

				w.Logger().Info().
					Uint32("entity", uint32(entity)).
					Msgf("Player %s received %d damage", command.Target, command.Damage)
			} else {
				player.Destroy()

				w.SendTo(cmd.Persona, event.PlayerDeath{Nickname: tag.Nickname})

				w.EmitSystemEvent(systemevent.PlayerDeath{Nickname: tag.Nickname})

				w.Logger().Info().Uint32("entity", uint32(entity)).Msgf("Player %s died", command.Target)
			}
		}
	}
}
