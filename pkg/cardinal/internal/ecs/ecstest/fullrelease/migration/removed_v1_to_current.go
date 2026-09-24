package migration

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/component"
)

// RemovedV1ToCurrent drops the field the current build no longer declares.
//
// The retired shape is where B is still readable, so a migration that needed the value could use
// it before it goes. This one does not, which is the common case: it exists so the restore has a
// route, not because the conversion is interesting.
type RemovedV1ToCurrent struct {
	Old ecs.In[component.RemovedV1]
	New ecs.Out[component.Removed]

	// Calls counts entities converted. See [RenamedV1ToCurrent].
	Calls int
}

func (m *RemovedV1ToCurrent) Migrate() {
	m.New.Set(component.Removed{A: m.Old.Get().A})
	m.Calls++
}
