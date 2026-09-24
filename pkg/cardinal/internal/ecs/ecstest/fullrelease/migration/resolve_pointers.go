package migration

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/component"
)

// UnresolvedSlot is the target a pointer gets when no entity holds its slot. Zero is not usable as
// a marker, since it is also a valid entity ID.
const UnresolvedSlot uint32 = 4294967295

// ResolvePointers is case 3c: it turns a reference by slot number into a reference by entity ID.
//
// It needs two whole-world reads, which is what makes this the case a per-entity migration cannot
// do at all. Nothing on the pointer's own entity says which entity holds a given slot; that fact
// exists only across the world.
type ResolvePointers struct {
	Pointers ecs.All[component.Pointer]
	Slots    ecs.All[component.Slotted]
	Write    ecs.Put[component.Pointer]

	// Calls counts how many times the whole-world pass ran this.
	Calls int
	// Unresolved counts pointers whose slot matched no entity.
	Unresolved int
}

func (m *ResolvePointers) Migrate() {
	m.Calls++

	bySlot := make(map[uint32]ecs.EntityID, m.Slots.Len())
	for id, slotted := range m.Slots.Iter() {
		bySlot[slotted.Number] = id
	}

	for id, pointer := range m.Pointers.Iter() {
		target, found := bySlot[pointer.Slot]
		if !found {
			// A dangling reference is reported rather than guessed at, and left visible in the
			// data so it can be found later.
			pointer.Target = UnresolvedSlot
			m.Unresolved++
			m.Write.Set(id, pointer)
			continue
		}
		pointer.Target = uint32(target)
		m.Write.Set(id, pointer)
	}
}
