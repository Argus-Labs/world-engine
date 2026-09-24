package migration

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/component"
)

// AddedDefault is the value the migration gives B on every entity that predates it. A real
// migration would derive it or take it from config; the fixture needs a number distinct from
// anything else so an assertion cannot pass by accident.
const AddedDefault uint32 = 100

// AddedV1ToCurrent fills in the field the current build added.
//
// Doing nothing would leave B at zero, which may even be correct — but the engine cannot know
// that, because zero is a legitimate value. Requiring the migration is what turns an unstated
// assumption into a line of code.
type AddedV1ToCurrent struct {
	Old ecs.In[component.AddedV1]
	New ecs.Out[component.Added]

	// Calls counts entities converted. See [RenamedV1ToCurrent].
	Calls int
}

func (m *AddedV1ToCurrent) Migrate() {
	m.New.Set(component.Added{A: m.Old.Get().A, B: AddedDefault})
	m.Calls++
}
