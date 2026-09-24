package migration

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/component"
)

// MovedV1ToCurrent is case 2d: a field moves from one surviving component to another.
//
// Two inputs and two outputs, and every one of the four components involved still exists after the
// migration. Both inputs are required: the value has to come from somewhere and has to land
// somewhere, and an entity holding only one of the pair could not be given a correct answer.
type MovedV1ToCurrent struct {
	From ecs.In[component.MovedFromV1]
	To   ecs.In[component.MovedToV1]

	NewFrom ecs.Out[component.MovedFrom]
	NewTo   ecs.Out[component.MovedTo]

	// Calls counts entities converted. See [RenamedV1ToCurrent].
	Calls int
}

func (m *MovedV1ToCurrent) Migrate() {
	from := m.From.Get()
	m.NewFrom.Set(component.MovedFrom{A: from.A})
	m.NewTo.Set(component.MovedTo{C: m.To.Get().C, B: from.B})
	m.Calls++
}
