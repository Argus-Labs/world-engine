package system

import (
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/component"
	systemevent "github.com/argus-labs/world-engine/pkg/template/basic/shards/game/system_event"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type GraveyardSystemState struct {
	cardinal.BaseSystemState
	PlayerDeathSystemEvents cardinal.WithSystemEventReceiver[systemevent.PlayerDeath]
	Graves                  GraveSearch
}

func GraveyardSystem(state *GraveyardSystemState) {
	for event := range state.PlayerDeathSystemEvents.Iter() {
		entity := state.Create[Grave]()
		entity.Set(component.Gravestone{Nickname: event.Nickname})

		state.Logger().Info().Msgf("Created grave stone for player %s", event.Nickname)
	}
}
