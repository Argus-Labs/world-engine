package system

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/examples/basic/component"
	systemevent "github.com/argus-labs/world-engine/pkg/cardinal/examples/basic/system_event"
)

type GraveyardSystemState struct {
	cardinal.BaseSystemState
	PlayerDeathSystemEvents cardinal.WithSystemEventReceiver[systemevent.PlayerDeath]
	Graves                  GraveSearch
}

func GraveyardSystem(state *GraveyardSystemState) error {
	for event := range state.PlayerDeathSystemEvents.Iter() {
		_, _ = state.Graves.Create(component.Gravestone{Nickname: event.Nickname})

		state.Logger().Info().Msgf("Created grave stone for player %s", event.Nickname)
	}
	return nil
}
