package system

import (
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/command"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/component"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/event"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type PlayerLeaveSystem struct{}

// PlayerLeaveSystem is called when a player leaves a quadrant (e.g. to join another quadrant).
func (s *PlayerLeaveSystem) Run(w *cardinal.World) {
	players := make(map[string]cardinal.Entity)

	for player := range w.Exact[Player]().Iter() {
		players[player.Get[component.PlayerTag]().ArgusAuthID] = player
	}

	for cmd := range w.Commands[command.PlayerLeave]() {
		command := cmd.Payload

		entity, exists := players[command.ArgusAuthID]
		if !exists {
			w.Logger().Info().Msgf("Player with ID %s not found", command.ArgusAuthID)
			continue
		}

		entity.Destroy()

		w.Broadcast(event.PlayerDeparture{
			ArgusAuthID: command.ArgusAuthID,
		})
	}
}
