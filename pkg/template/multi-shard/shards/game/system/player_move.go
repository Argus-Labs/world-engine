package system

import (
	"time"

	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/command"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/component"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/event"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type MovePlayerSystem struct{}

func (s *MovePlayerSystem) Run(w *cardinal.World) {
	players := w.Exact[Player]()
	for cmd := range w.Commands[command.MovePlayer]() {
		command := cmd.Payload

		for player := range players.Iter() {
			entity := player.ID()
			tag := player.Get[component.PlayerTag]()

			if command.ArgusAuthID != tag.ArgusAuthID {
				continue
			}

			isOnline := player.Get[component.OnlineStatus]().Online

			if !isOnline {
				w.Broadcast(event.PlayerSpawn{
					ArgusAuthID:   tag.ArgusAuthID,
					ArgusAuthName: tag.ArgusAuthName,
					X:             command.X,
					Y:             command.Y,
				})
			}

			player.Set(component.Position{X: int(command.X), Y: int(command.Y)})
			player.Set(component.OnlineStatus{Online: true, LastActive: time.Now()})

			w.Broadcast(event.PlayerMovement{
				ArgusAuthID: tag.ArgusAuthID,
				X:           command.X,
				Y:           command.Y,
			})

			name := tag.ArgusAuthName

			w.Logger().Info().
				Uint32("entity", uint32(entity)).
				Msgf("Player %s (id: %s) moved to %d, %d", name, tag.ArgusAuthID, command.X, command.Y)
		}
	}
}
