package migration

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/component"
)

// MergedFromV1 is case 2b: two components the current build no longer declares become one that it
// does.
//
// Two required inputs, which is what the order exists for. Neither stored value can be carried
// over on its own, and the migration cannot run until both have been read, so the engine has to
// know it needs both before it starts rather than discovering it half way.
//
// Both are required rather than optional because merged has no sensible meaning with only half its
// data. Case 2c takes the other view, and says so with [ecs.Optional].
type MergedFromV1 struct {
	Left  ecs.In[component.MergeLeftV1]
	Right ecs.In[component.MergeRightV1]

	New ecs.Out[component.Merged]

	// Calls counts entities converted. See [RenamedV1ToCurrent].
	Calls int
}

func (m *MergedFromV1) Migrate() {
	m.New.Set(component.Merged{A: m.Left.Get().A, B: m.Right.Get().B})
	m.Calls++
}
