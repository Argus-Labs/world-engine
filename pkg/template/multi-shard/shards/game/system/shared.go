package system

import (
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/component"
)

// Player is the archetype of player entities.
type Player struct {
	Tag      component.PlayerTag
	Position component.Position
	Online   component.OnlineStatus
}
