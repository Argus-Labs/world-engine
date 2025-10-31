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
		OnlineStatus cardinal.Ref[component.OnlineStatus]
		PlayerTag    cardinal.Ref[component.PlayerTag]
	}]
	PlayerDepartureEvent cardinal.WithEvent[event.PlayerDeparture]
}

func OnlineStatusUpdater(state *OnlineStatusUpdaterState) error {
	for entity, player := range state.Players.Iter() {
		isOnline := player.OnlineStatus.Get().Online
		lastActive := player.OnlineStatus.Get().LastActive

		// If the player has not been active for 5 minutes, set them to offline
		if isOnline && time.Since(lastActive) > 5*time.Minute {
			player.OnlineStatus.Set(component.OnlineStatus{Online: false, LastActive: lastActive})

			state.PlayerDepartureEvent.Emit(event.PlayerDeparture{
				ArgusAuthID: player.PlayerTag.Get().ArgusAuthID,
			})

			state.Logger().Info().
				Uint32("entity", uint32(entity)).
				Msgf("Player %s (id: %s) is offline", player.PlayerTag.Get().ArgusAuthName, player.PlayerTag.Get().ArgusAuthID)
		}
	}
	return nil
}
