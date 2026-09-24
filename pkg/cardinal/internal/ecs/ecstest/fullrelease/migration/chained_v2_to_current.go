package migration

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/component"
)

// ChainedV2ToCurrent is the second step of the chain: it drops A and derives C from it before it
// goes.
//
// Its input is the previous step's output, not anything a save holds. The engine has to run the two
// in order, and that order comes from the shapes they declare — this one's input is that one's
// output — not from the order they were registered in.
type ChainedV2ToCurrent struct {
	Old ecs.In[component.ChainedV2]
	New ecs.Out[component.Chained]

	// Calls counts entities converted, for the same reason RenamedV1ToCurrent keeps one. See [RenamedV1ToCurrent].
	Calls int
}

func (m *ChainedV2ToCurrent) Migrate() {
	old := m.Old.Get()
	m.New.Set(component.Chained{B: old.B, C: old.A + old.B})
	m.Calls++
}
