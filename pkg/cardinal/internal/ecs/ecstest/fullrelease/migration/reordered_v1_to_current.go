package migration

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/component"
)

// ReorderedV1ToCurrent puts each value back in the field that now holds it.
//
// Both structs carry the same two names and the same two wire types, so the stored bytes decode
// into the current struct without error — with A's value in B and B's in A. The migration reads
// through the retired shape, where field 1 is still A, and assigns by name.
type ReorderedV1ToCurrent struct {
	Old ecs.In[component.ReorderedV1]
	New ecs.Out[component.Reordered]

	// Calls counts entities converted. See [RenamedV1ToCurrent].
	Calls int
}

func (m *ReorderedV1ToCurrent) Migrate() {
	old := m.Old.Get()
	m.New.Set(component.Reordered{A: old.A, B: old.B})
	m.Calls++
}
