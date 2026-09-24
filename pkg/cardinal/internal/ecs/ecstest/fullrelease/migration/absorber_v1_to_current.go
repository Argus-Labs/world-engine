package migration

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/component"
)

// AbsorbedDefault is what B becomes on an entity that never had an absorbed component. A real
// migration would pick something meaningful; the fixture needs a number nothing else uses.
const AbsorbedDefault uint32 = 200

// AbsorberV1ToCurrent is case 2c: a component that already existed takes over another one's field.
//
// The second input is optional, and that is the whole difference from 2b. An entity may well have
// an absorber and never have had an absorbed, and it still has to migrate — its absorber is its
// own data and cannot simply be refused. A required input would skip that entity and leave its
// stored absorber with nothing to consume it, which the restore reports as an error.
//
// Optional does not mean unordered. If anything produced an absorbed shape, it would still have to
// run before this, because an optional input the engine could have filled but did not is a wrong
// answer rather than a missing one.
type AbsorberV1ToCurrent struct {
	Old ecs.In[component.AbsorberV1]
	Eat ecs.Optional[component.AbsorbedV1]

	New ecs.Out[component.Absorber]

	// Calls counts entities converted. See [RenamedV1ToCurrent].
	Calls int
	// WithAbsorbed counts the subset that actually had one, so a test can tell an absent optional
	// from one that was present and happened to hold the default.
	WithAbsorbed int
}

func (m *AbsorberV1ToCurrent) Migrate() {
	m.Calls++

	eaten, had := m.Eat.Get()
	if !had {
		m.New.Set(component.Absorber{A: m.Old.Get().A, B: AbsorbedDefault})
		return
	}

	m.WithAbsorbed++
	m.New.Set(component.Absorber{A: m.Old.Get().A, B: eaten.B})
}
