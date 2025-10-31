package system

import (
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/component"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type RegenSystemState struct {
	cardinal.BaseSystemState
	cardinal.Contains[struct {
		cardinal.Ref[component.Health]
	}]
}

func RegenSystem(state *RegenSystemState) error {
	for _, health := range state.Iter() { // Another shorthand
		health.Set(component.Health{HP: health.Get().HP + 10})
	}
	return nil
}
