package system

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/component"
)

type PlayerSetUpdaterState struct {
	cardinal.BaseSystemState
}

// PlayerSetUpdater updates the playerSet with all players in the world state.
func PlayerSetUpdater(state *PlayerSetUpdaterState) {
	playerSet.Clear()
	for player := range state.Exact[Player]().Iter() {
		playerSet.Add(player.Get[component.PlayerTag]().ArgusAuthID)
	}
}
