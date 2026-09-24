package migration

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/component"
)

// SplitV1ToCurrent splits one stored component into two.
//
// Two Out fields, one In. Both outputs must be set or the restore fails, and the entity ends up in
// a different archetype than its save described, since it now holds a component the snapshot never
// listed.
type SplitV1ToCurrent struct {
	Old ecs.In[component.SplitV1]

	New ecs.Out[component.Split]
	Off ecs.Out[component.SplitOff]

	// Calls counts entities converted. See [RenamedV1ToCurrent].
	Calls int
}

func (m *SplitV1ToCurrent) Migrate() {
	old := m.Old.Get()
	m.New.Set(component.Split{A: old.A})
	m.Off.Set(component.SplitOff{B: old.B})
	m.Calls++
}
