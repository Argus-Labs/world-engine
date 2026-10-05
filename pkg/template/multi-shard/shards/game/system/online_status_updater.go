package system

import (
	"time"

	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/component"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/event"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

// onlinePlayer matches every entity carrying an online status and a player tag, whatever else
// it holds.
type onlinePlayer struct {
	OnlineStatus component.OnlineStatus
	PlayerTag    component.PlayerTag
}

type OnlineStatusUpdater struct{}

func (s *OnlineStatusUpdater) Run(w *cardinal.World) {
	for player := range w.Contains[onlinePlayer]().Iter() {
		entity := player.ID()
		status := player.Get[component.OnlineStatus]()
		isOnline := status.Online
		lastActive := status.LastActive

		// If the player has not been active for 5 minutes, set them to offline
		if isOnline && time.Since(lastActive) > 5*time.Minute {
			player.Set(component.OnlineStatus{Online: false, LastActive: lastActive})
			tag := player.Get[component.PlayerTag]()

			w.Broadcast(event.PlayerDeparture{
				ArgusAuthID: tag.ArgusAuthID,
			})

			w.Logger().Info().
				Uint32("entity", uint32(entity)).
				Msgf("Player %s (id: %s) is offline", tag.ArgusAuthName, tag.ArgusAuthID)
		}
	}
}
