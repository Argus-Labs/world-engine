package migration

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/component"
)

// ChainedV1ToV2 is the first step of the chain: it derives B from A.
//
// Its output is a retired shape, not a registered component, so nothing it produces can land on an
// entity. The only thing that can consume it is another migration, which is what makes this a chain
// rather than two independent conversions.
type ChainedV1ToV2 struct {
	Old ecs.In[component.ChainedV1]
	New ecs.Out[component.ChainedV2]

	// Calls counts entities converted, for the same reason RenamedV1ToCurrent keeps one. See [RenamedV1ToCurrent].
	Calls int
}

func (m *ChainedV1ToV2) Migrate() {
	old := m.Old.Get()
	m.New.Set(component.ChainedV2{A: old.A, B: old.A * chainedBFactor})
	m.Calls++
}

// chainedBFactor is arbitrary. It exists so B is visibly derived from A rather than copied, which
// makes a step that silently did not run show up in the result.
const chainedBFactor = 2
