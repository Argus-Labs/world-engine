package migration

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/component"
)

// PointerV1ToCurrent keeps the slot and leaves the target unresolved.
//
// Keeping the slot is the point: resolving it needs every slotted entity, which only the
// whole-world pass can see, so the per-entity migration has to hand the question on rather than
// answer it.
type PointerV1ToCurrent struct {
	Old ecs.In[component.PointerV1]
	New ecs.Out[component.Pointer]

	// Calls counts entities converted. See [RenamedV1ToCurrent].
	Calls int
}

func (m *PointerV1ToCurrent) Migrate() {
	m.New.Set(component.Pointer{Slot: m.Old.Get().Slot})
	m.Calls++
}
