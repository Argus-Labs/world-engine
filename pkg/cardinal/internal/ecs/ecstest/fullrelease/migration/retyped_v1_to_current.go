package migration

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/component"
)

// RetypedV1ToCurrent converts the integer the old build stored into the float the current one
// declares.
//
// This is the one case where skipping the migration cannot quietly produce a wrong number: the
// wire types differ, so the current struct would fail to decode the stored bytes at all. The
// retired shape is the only thing that can read them.
type RetypedV1ToCurrent struct {
	Old ecs.In[component.RetypedV1]
	New ecs.Out[component.Retyped]

	// Calls counts entities converted. See [RenamedV1ToCurrent].
	Calls int
}

func (m *RetypedV1ToCurrent) Migrate() {
	m.New.Set(component.Retyped{A: float32(m.Old.Get().A)})
	m.Calls++
}
