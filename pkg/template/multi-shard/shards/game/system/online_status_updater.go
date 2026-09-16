package system

import (
	"time"

	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/component"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/event"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type OnlineStatusUpdaterState struct {
	cardinal.BaseSystemState
	Players cardinal.Contains[struct {
		OnlineStatus cardinal.WithComponent[component.OnlineStatus]
		PlayerTag    cardinal.WithComponent[component.PlayerTag]
	}]
	PlayerDepartureEvent cardinal.WithEvent[event.PlayerDeparture]
}

func OnlineStatusUpdater(state *OnlineStatusUpdaterState) {
	for entity, player := range state.Players.Iter() {
		isOnline := player.Get[component.OnlineStatus]().Online
		lastActive := player.Get[component.OnlineStatus]().LastActive

		// If the player has not been active for 5 minutes, set them to offline
		if isOnline && time.Since(lastActive) > 5*time.Minute {
			player.Set(component.OnlineStatus{Online: false, LastActive: lastActive})
			tag := player.Get[component.PlayerTag]()

			state.PlayerDepartureEvent.Broadcast(event.PlayerDeparture{
				ArgusAuthID: tag.ArgusAuthID,
			})

			state.Logger().Info().
				Uint32("entity", uint32(entity)).
				Msgf("Player %s (id: %s) is offline", tag.ArgusAuthName, tag.ArgusAuthID)
		}
	}
}
