package system

import (
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/component"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type Player struct {
	Tag    cardinal.WithComponent[component.PlayerTag]
	Health cardinal.WithComponent[component.Health]
}

type PlayerSearch = cardinal.Exact[Player]

type Grave struct {
	Grave cardinal.WithComponent[component.Gravestone]
}

type GraveSearch = cardinal.Exact[Grave]
