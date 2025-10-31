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

func GraveyardSystem(state *GraveyardSystemState) error {
	for event := range state.PlayerDeathSystemEvents.Iter() {
		_, entity := state.Graves.Create()
		entity.Grave.Set(component.Gravestone{Nickname: event.Nickname})

		state.Logger().Info().Msgf("Created grave stone for player %s", event.Nickname)
	}
	return nil
}
