package ecs

import (
	"fmt"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/entity"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	types "github.com/argus-labs/world-engine/cardinal/ecs/storage/types/v1"
)

// WorldId is a unique identifier for a world.
type WorldId int

type Components interface {
	// GetComponent gets the component from the entry.
	GetComponent(*types.Entry, string) (component.IComponentType, error)
	// SetComponent updates the component data for the entry.
	SetComponent(*types.Entry, component.IComponentType) error
}

// World is a collection of entities and components.
type World interface {
	Components
	// ID returns the unique identifier for the world.
	ID() WorldId
	// Create creates a new entity with the specified componentStore.
	Create(components ...component.IComponentType) (storage.Entity, error)
	// CreateMany creates a new entity with the specified componentStore.
	CreateMany(n int, components ...component.IComponentType) ([]storage.Entity, error)
	// Entry returns an entry for the specified entity.
	Entry(storage.Entity) (*types.Entry, error)
	// Remove removes the specified entity.
	Remove(storage.Entity) error
	// Valid returns true if the specified entity is valid.
	Valid(storage.Entity) (bool, error)
	// NumEntities returns the number of entities in the world.
	NumEntities() (int, error)
	// Update loops through and executes all the systems in the world
	Update()
	// AddSystem adds a system to the world.
	AddSystem(System)
	// RegisterComponents registers the components in the world.
	RegisterComponents(...component.IComponentType)
}

var _ World = &world{}

type world struct {
	id      WorldId
	store   storage.WorldStorage
	systems []System
	tr      storage.TypeRegistry
}

// NewWorld creates a new world.
func NewWorld(s storage.WorldStorage) World {
	// TODO: this should prob be handled by redis as well...
	worldId := nextWorldId
	nextWorldId++
	w := &world{
		id:      worldId,
		store:   s,
		tr:      storage.NewTypeRegistry(),
		systems: make([]System, 0, 256), // this can just stay in memory.
	}
	return w
}

func (w *world) GetComponent(entry *types.Entry, componentID string) (component.IComponentType, error) {
	s := w.store.CompStore.StorageFromID(componentID)
	comp, err := s.Component(storage.ArchetypeIndex(entry.Location.ArchetypeIndex), storage.ComponentIndex(entry.Location.ComponentIndex))
	return comp, err
}

func (w *world) SetComponent(entry *types.Entry, componentType component.IComponentType) error {
	ai, ci := entry.Location.ArchetypeIndex, entry.Location.ComponentIndex
	comp := componentType.ProtoReflect().New().(component.IComponentType)
	err := w.store.CompStore.
		StorageFromID(component.ID(componentType)).
		SetComponent(storage.ArchetypeIndex(ai), storage.ComponentIndex(ci), comp)
	return err
}

func (w *world) AddSystem(s System) {
	w.systems = append(w.systems, s)
}

func (w *world) Update() {
	for _, sys := range w.systems {
		sys(w)
	}
}

func (w *world) RegisterComponents(ct ...component.IComponentType) {
	for _, c := range ct {
		w.tr.Register(c)
	}
}

var nextWorldId WorldId = 0

func (w *world) ID() WorldId {
	return w.id
}

func (w *world) CreateMany(num int, components ...component.IComponentType) ([]storage.Entity, error) {
	archetypeIndex, err := w.getArchetypeForComponents(components)
	if err != nil {
		return nil, err
	}
	entities := make([]storage.Entity, 0, num)
	for i := 0; i < num; i++ {
		e, err := w.createEntity(archetypeIndex)
		if err != nil {
			return nil, err
		}

		entities = append(entities, e)
	}
	return entities, nil
}

func (w *world) Create(components ...component.IComponentType) (storage.Entity, error) {
	archetypeIndex, err := w.getArchetypeForComponents(components)
	if err != nil {
		return 0, err
	}
	return w.createEntity(archetypeIndex)
}

func (w *world) createEntity(archetypeIndex storage.ArchetypeIndex) (storage.Entity, error) {
	newEntity, err := w.nextEntity()
	if err != nil {
		return 0, err
	}
	archetype, err := w.store.ArchAccessor.Archetype(archetypeIndex)
	if err != nil {
		return 0, err
	}
	componentIndex, err := w.store.CompStore.PushRawComponents(archetype.Components, archetypeIndex)
	if err != nil {
		return 0, err
	}
	err = w.store.EntityLocStore.Insert(newEntity.ID(), archetypeIndex, componentIndex)
	if err != nil {
		return 0, err
	}
	err = w.store.ArchAccessor.PushEntity(storage.ArchetypeIndex(archetype.ArchetypeIndex), newEntity)
	if err != nil {
		return 0, err
	}
	err = w.createEntry(newEntity)
	return newEntity, err
}

func (w *world) createEntry(e storage.Entity) error {
	id := e.ID()
	loc, err := w.store.EntityLocStore.Location(id)
	if err != nil {
		return err
	}
	entry := storage.NewEntry(id, loc)
	return w.store.EntryStore.SetEntry(entry)
}

func (w *world) Valid(e storage.Entity) (bool, error) {
	if e == storage.Null {
		return false, nil
	}
	ok, err := w.store.EntityLocStore.ContainsEntity(e.ID())
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	loc, err := w.store.EntityLocStore.Location(e.ID())
	if err != nil {
		return false, err
	}
	a := loc.ArchetypeIndex
	c := loc.ComponentIndex
	// If the version of the entity is not the same as the version of the archetype,
	// the entity is invalid (it means the entity is already destroyed).\
	arch, _ := w.store.ArchAccessor.Archetype(storage.ArchetypeIndex(a))
	eid := storage.Entity(arch.EntityIds[c])
	return loc.Valid && e == eid, nil
}

func (w *world) Entry(entity storage.Entity) (*types.Entry, error) {
	id := entity.ID()
	entry, err := w.store.EntryStore.GetEntry(id)
	if err != nil {
		return nil, err
	}
	err = w.store.EntryStore.SetEntity(id, entity)
	if err != nil {
		return nil, err
	}
	loc, err := w.store.EntityLocStore.Location(id)
	if err != nil {
		return nil, err
	}
	err = w.store.EntryStore.SetLocation(id, loc)
	if err != nil {
		return nil, err
	}
	return entry, nil
}

func (w *world) NumEntities() (int, error) {
	l, err := w.store.EntityLocStore.Len()
	if err != nil {
		return 0, err
	}
	return l, nil
}

func (w *world) Remove(ent storage.Entity) error {
	ok, err := w.Valid(ent)
	if err != nil {
		return err
	}
	if ok {
		loc, err := w.store.EntityLocStore.Location(ent.ID())
		if err != nil {
			return err
		}
		if err := w.store.EntityLocStore.Remove(ent.ID()); err != nil {
			return err
		}
		if err := w.removeAtLocation(ent, loc); err != nil {
			return err
		}
	}
	return nil
}

func (w *world) removeAtLocation(ent storage.Entity, loc *types.Location) error {
	archIndex := loc.ArchetypeIndex
	componentIndex := loc.ComponentIndex
	_, err := w.store.ArchAccessor.RemoveEntityAt(storage.ArchetypeIndex(archIndex), int(componentIndex))
	if err != nil {
		return err
	}
	archetype, _ := w.store.ArchAccessor.Archetype(storage.ArchetypeIndex(archIndex))
	err = w.store.CompStore.Remove(storage.ArchetypeIndex(archIndex), archetype.Components, storage.ComponentIndex(componentIndex))
	if err != nil {
		return err
	}
	if int(componentIndex) < len(archetype.EntityIds) {
		swapped := archetype.EntityIds[componentIndex]
		if err := w.store.EntityLocStore.Set(entity.ID(swapped), loc); err != nil {
			return err
		}
	}
	w.store.EntityMgr.Destroy(ent.IncVersion())
	return nil
}

func (w *world) nextEntity() (storage.Entity, error) {
	return w.store.EntityMgr.NewEntity()
}

func (w *world) insertArchetype(comps []component.IComponentType) (storage.ArchetypeIndex, error) {
	w.store.ArchCompIdxStore.Push(comps)
	archIndex, err := w.store.ArchAccessor.GetNextArchetypeIndex()
	if err != nil {
		return 0, err
	}

	err = w.store.ArchAccessor.PushArchetype(storage.ArchetypeIndex(archIndex), comps)
	return storage.ArchetypeIndex(archIndex), err
}

func (w *world) getArchetypeForComponents(components []component.IComponentType) (storage.ArchetypeIndex, error) {
	if len(components) == 0 {
		panic("entity must have at least one component")
	}
	if ii := w.store.ArchCompIdxStore.Search(filter.Exact(components)); ii.HasNext() {
		return ii.Next(), nil
	}
	if !w.noDuplicates(components) {
		panic(fmt.Sprintf("duplicate component types: %v", components))
	}
	ai, err := w.insertArchetype(components)
	return ai, err
}

func (w *world) noDuplicates(components []component.IComponentType) bool {
	// check if there are duplicate values inside componentStore slice
	for i := 0; i < len(components); i++ {
		for j := i + 1; j < len(components); j++ {
			if components[i] == components[j] {
				return false
			}
		}
	}
	return true
}
