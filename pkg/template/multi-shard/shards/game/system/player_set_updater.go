package system

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type PlayerSetUpdaterState struct {
	cardinal.BaseSystemState
	Players PlayerSearch
}

// PlayerSetUpdater updates the playerSet with all players in the world state.
func PlayerSetUpdater(state *PlayerSetUpdaterState) error {
	playerSet.Clear()
	for _, player := range state.Players.Iter() {
		playerSet.Add(player.Tag.Get().ArgusAuthID)
	}
	return nil
}
