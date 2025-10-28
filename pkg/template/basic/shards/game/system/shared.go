package system

import (
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/component"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type PlayerSearch = cardinal.Exact[struct {
	Tag    cardinal.Ref[component.PlayerTag]
	Health cardinal.Ref[component.Health]
}]

type GraveSearch = cardinal.Exact[struct {
	Grave cardinal.Ref[component.Gravestone]
}]
