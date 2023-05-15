package storage

import (
	"github.com/argus-labs/world-engine/cardinal/ecs/component"
)

type ArchetypeIndex int

var _ ArchetypeAccessor = &archetypeStorageImpl{}

func NewArchetypeAccessor() ArchetypeAccessor {
	return &archetypeStorageImpl{archs: make([]*Archetype, 0)}
}

type archetypeStorageImpl struct {
	archs []*Archetype
}

func (a *archetypeStorageImpl) PushArchetype(index ArchetypeIndex, layout *Layout) {
	a.archs = append(a.archs, &Archetype{
		Index:      index,
		Entitys:    make([]EntityID, 0, 256),
		ArchLayout: layout,
	})
}

func (a archetypeStorageImpl) Count() int {
	return len(a.archs)
}

func (a archetypeStorageImpl) Archetype(index ArchetypeIndex) ArchetypeStorage {
	return a.archs[index]
}

// Archetype is a collection of Entities for a specific archetype of components.
// This structure allows to quickly find Entities based on their components.
type Archetype struct {
	Index      ArchetypeIndex
	Entitys    []EntityID
	ArchLayout *Layout
}

var _ ArchetypeStorage = &Archetype{}

// NewArchetype creates a new archetype.
func NewArchetype(index ArchetypeIndex, layout *Layout) *Archetype {
	return &Archetype{
		Index:      index,
		Entitys:    make([]EntityID, 0, 256),
		ArchLayout: layout,
	}
}

// Layout is a collection of archetypes for a specific ArchLayout of components.
func (archetype *Archetype) Layout() *Layout {
	return archetype.ArchLayout
}

// Entities returns all Entities in this archetype.
func (archetype *Archetype) Entities() []EntityID {
	return archetype.Entitys
}

// SwapRemove removes an Ent from the archetype and returns it.
func (archetype *Archetype) SwapRemove(entityIndex ComponentIndex) EntityID {
	removed := archetype.Entitys[entityIndex]
	archetype.Entitys[entityIndex] = archetype.Entitys[len(archetype.Entitys)-1]
	archetype.Entitys = archetype.Entitys[:len(archetype.Entitys)-1]
	return removed
}

// LayoutMatches returns true if the given ArchLayout matches this archetype.
func (archetype *Archetype) LayoutMatches(components []component.IComponentType) bool {
	if len(archetype.ArchLayout.Components()) != len(components) {
		return false
	}
	for _, componentType := range components {
		if !archetype.ArchLayout.HasComponent(componentType) {
			return false
		}
	}
	return true
}

// PushEntity adds an Ent to the archetype.
func (archetype *Archetype) PushEntity(id EntityID) {
	archetype.Entitys = append(archetype.Entitys, id)
}

// Count returns the number of Entitys in the archetype.
func (archetype *Archetype) Count() int {
	return len(archetype.Entitys)
}
