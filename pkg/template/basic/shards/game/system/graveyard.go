package system

import (
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/component"
	systemevent "github.com/argus-labs/world-engine/pkg/template/basic/shards/game/system_event"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type GraveyardSystem struct{}

func (s *GraveyardSystem) Run(w *cardinal.World) {
	for event := range w.SystemEvents[systemevent.PlayerDeath]() {
		entity := w.Create[Grave]()
		entity.Set(component.Gravestone{Nickname: event.Nickname})

		w.Logger().Info().Msgf("Created grave stone for player %s", event.Nickname)
	}
}
