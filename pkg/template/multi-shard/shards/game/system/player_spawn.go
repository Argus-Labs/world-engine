package system

import (
	"fmt"
	"time"

	chatcommand "github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/command"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/command"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/component"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/event"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type PlayerSpawnSystem struct {
	ChatWorld cardinal.OtherWorld
}

func (s *PlayerSpawnSystem) Run(w *cardinal.World) {
	// Index the players already in the world so a repeated spawn reuses the existing entity.
	// Reading the world each tick keeps this correct after a snapshot restore.
	existing := make(map[string]bool)
	for player := range w.Exact[Player]().Iter() {
		existing[player.Get[component.PlayerTag]().ArgusAuthID] = true
	}

	for cmd := range w.Commands[command.PlayerSpawn]() {
		command := cmd.Payload

		// Regardless of whether the player exists or not, we emit a spawn event
		// Because the act of spawning is also creating (if they don’t already exist)
		w.Broadcast(event.PlayerSpawn{
			ArgusAuthID:   command.ArgusAuthID,
			ArgusAuthName: command.ArgusAuthName,
			X:             command.X,
			Y:             command.Y,
		})

		if existing[command.ArgusAuthID] {
			w.Logger().Info().Msgf("Player with ID %s already exists, skipping creation", command.ArgusAuthID)
			continue
		}

		player := w.Create[Player]()

		id := player.ID()
		player.Set(component.PlayerTag{ArgusAuthID: command.ArgusAuthID, ArgusAuthName: command.ArgusAuthName})
		player.Set(component.Position{X: int(command.X), Y: int(command.Y)})
		player.Set(component.OnlineStatus{Online: true, LastActive: time.Now()})

		existing[command.ArgusAuthID] = true

		w.Logger().Info().
			Uint32("entity", uint32(id)).
			Msgf("Created player %s (id: %s)", command.ArgusAuthName, command.ArgusAuthID)

		// Inform chat shard about the spawn
		w.SendToShard(s.ChatWorld, chatcommand.UserChat{
			ArgusAuthID:   command.ArgusAuthID,
			ArgusAuthName: command.ArgusAuthName,
			Message:       fmt.Sprintf("%s joined at (%s)", command.ArgusAuthName, w.Timestamp().Format(time.RFC3339)),
		})
	}
}
