package system

import (
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/component"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type RegenSystemState struct {
	cardinal.BaseSystemState
}

func RegenSystem(state *RegenSystemState) {
	// Contains matches every entity with Health, whatever else it carries.
	for health := range state.Contains[struct{ component.Health }]().Iter() {
		health.Set(component.Health{HP: health.Get[component.Health]().HP + 10})
	}
}
