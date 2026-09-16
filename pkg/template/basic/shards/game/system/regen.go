package system

import (
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/component"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type RegenSystemState struct {
	cardinal.BaseSystemState
	cardinal.Contains[struct {
		cardinal.WithComponent[component.Health]
	}]
}

func RegenSystem(state *RegenSystemState) {
	for _, health := range state.Iter() { // Another shorthand
		health.Set(component.Health{HP: health.Get[component.Health]().HP + 10})
	}
}
