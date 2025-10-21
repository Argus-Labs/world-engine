package system

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/examples/rampage/component"
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
