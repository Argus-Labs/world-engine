package system

import (
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/component"
)

// Player is the archetype of player entities, matched by w.Exact[Player]().
type Player struct {
	Tag    component.PlayerTag
	Health component.Health
}

// Grave is the archetype of gravestone entities.
type Grave struct {
	Grave component.Gravestone
}
