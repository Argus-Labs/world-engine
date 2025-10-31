package system

import (
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/component"
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/event"
	systemevent "github.com/argus-labs/world-engine/pkg/template/basic/shards/game/system_event"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type AttackPlayerCommand struct {
	cardinal.BaseCommand
	Target string
	Damage uint32
}

func (a AttackPlayerCommand) Name() string {
	return "attack-player"
}

type AttackPlayerSystemState struct {
	cardinal.BaseSystemState
	AttackPlayerCommands    cardinal.WithCommand[AttackPlayerCommand]
	PlayerDeathSystemEvents cardinal.WithSystemEventEmitter[systemevent.PlayerDeath]
	PlayerDeathEvents       cardinal.WithEvent[event.PlayerDeath]
	Players                 PlayerSearch
}

func AttackPlayerSystem(state *AttackPlayerSystemState) error {
	for cmd := range state.AttackPlayerCommands.Iter() {
		command := cmd.Payload()
		for entity, player := range state.Players.Iter() {
			tag := player.Tag.Get()

			if command.Target != tag.Nickname {
				continue
			}

			newHealth := player.Health.Get().HP - int(command.Damage)
			if newHealth > 0 {
				player.Health.Set(component.Health{HP: newHealth})

				state.Logger().Info().
					Uint32("entity", uint32(entity)).
					Msgf("Player %s received %d damage", command.Target, command.Damage)
			} else {
				state.Players.Destroy(entity)

				state.PlayerDeathEvents.Emit(event.PlayerDeath{Nickname: tag.Nickname})

				state.PlayerDeathSystemEvents.Emit(systemevent.PlayerDeath{Nickname: tag.Nickname})

				state.Logger().Info().Uint32("entity", uint32(entity)).Msgf("Player %s died", command.Target)
			}
		}
	}
	return nil
}
