package cardinal

import (
	"fmt"

	"github.com/argus-labs/cardinal/component"
	"github.com/argus-labs/cardinal/filter"
	"github.com/argus-labs/cardinal/internal/entity"
	"github.com/argus-labs/cardinal/internal/storage"
)

// WorldId is a unique identifier for a world.
type WorldId int

// World is a collection of entities and components.
type World interface {
	// ID returns the unique identifier for the world.
	ID() WorldId
	// Create creates a new entity with the specified components.
	Create(components ...component.IComponentType) (Entity, error)
	// CreateMany creates a new entity with the specified components.
	CreateMany(n int, components ...component.IComponentType) ([]Entity, error)
	// Entry returns an entry for the specified entity.
	Entry(entity Entity) *Entry
	// Remove removes the specified entity.
	Remove(entity Entity)
	// Valid returns true if the specified entity is valid.
	Valid(e Entity) bool
	// Len returns the number of entities in the world.
	Len() int
	// StorageAccessor returns an accessor for the world's storage.
	// It is used to access components and archetypes by queries.
	StorageAccessor() StorageAccessor
	// Archetypes returns the archetypes in the world.
	Archetypes() []*storage.Archetype
	// Update loops through and executes all the systems in the world
	Update()
	// AddSystem adds a system to the world.
	AddSystem(System)
}

// StorageAccessor is an accessor for the world's storage.
type StorageAccessor struct {
	// Index is the search index for the world.
	Index *storage.Index
	// Components is the component storage for the world.
	Components *storage.Components
	// Archetypes is the archetype storage for the world.
	Archetypes []*storage.Archetype
}

type initializer func(w World)

type world struct {
	id           WorldId
	index        *storage.Index
	entities     *storage.LocationMap
	components   *storage.Components
	archetypes   []*storage.Archetype
	destroyed    []Entity
	entries      []*Entry
	nextEntityId entity.ID
	systems      []System
}

func (w *world) AddSystem(s System) {
	w.systems = append(w.systems, s)
}

func (w *world) Update() {
	for _, sys := range w.systems {
		sys(w)
	}
}

var nextWorldId WorldId = 0

var registeredInitializers []initializer

// RegisterInitializer registers an initializer for a world.
func RegisterInitializer(initializer initializer) {
	registeredInitializers = append(registeredInitializers, initializer)
}

// NewWorld creates a new world.
func NewWorld() World {
	worldId := nextWorldId
	nextWorldId++
	w := &world{
		id:           worldId,
		index:        storage.NewIndex(),
		entities:     storage.NewLocationMap(),
		components:   storage.NewComponents(),
		archetypes:   make([]*storage.Archetype, 0),
		destroyed:    make([]Entity, 0, 256),
		entries:      make([]*Entry, 1, 256),
		systems:      make([]System, 0, 256),
		nextEntityId: 1,
	}
	for _, initializer := range registeredInitializers {
		initializer(w)
	}
	return w
}

func (w *world) ID() WorldId {
	return w.id
}

func (w *world) CreateMany(num int, components ...component.IComponentType) ([]Entity, error) {
	archetypeIndex := w.getArchetypeForComponents(components)
	entities := make([]Entity, 0, num)
	for i := 0; i < num; i++ {
		e, err := w.createEntity(archetypeIndex)
		if err != nil {
			return nil, err
		}

		entities = append(entities, e)
	}
	return entities, nil
}

func (w *world) Create(components ...component.IComponentType) (Entity, error) {
	archetypeIndex := w.getArchetypeForComponents(components)
	return w.createEntity(archetypeIndex)
}

func (w *world) createEntity(archetypeIndex storage.ArchetypeIndex) (Entity, error) {
	nextEntity := w.nextEntity()
	archetype := w.archetypes[archetypeIndex]
	componentIndex, err := w.components.PushComponents(archetype.Layout().Components(), archetypeIndex)
	if err != nil {
		return 0, err
	}
	w.entities.Insert(nextEntity.ID(), archetypeIndex, componentIndex)
	archetype.PushEntity(nextEntity)
	w.createEntry(nextEntity)
	return nextEntity, nil
}

func (w *world) createEntry(e Entity) {
	id := e.ID()
	if int(id) >= len(w.entries) {
		w.entries = append(w.entries, nil)
	}
	loc := w.entities.Location(id)
	entry := &Entry{
		id:     id,
		entity: e,
		loc:    loc,
		World:  w,
	}
	w.entries[id] = entry
}

func (w *world) Valid(e Entity) bool {
	if e == Null {
		return false
	}
	if !w.entities.Contains(e.ID()) {
		return false
	}
	loc := w.entities.LocationMap[e.ID()]
	a := loc.Archetype
	c := loc.Component
	// If the version of the entity is not the same as the version of the archetype,
	// the entity is invalid (it means the entity is already destroyed).
	return loc.Valid && e == w.archetypes[a].Entities()[c]
}

func (w *world) Entry(entity Entity) *Entry {
	id := entity.ID()
	entry := w.entries[id]
	entry.entity = entity
	entry.loc = w.entities.LocationMap[id]
	return entry
}

func (w *world) Len() int {
	return w.entities.Len
}

func (w *world) Remove(ent Entity) {
	if w.Valid(ent) {
		loc := w.entities.LocationMap[ent.ID()]
		w.entities.Remove(ent.ID())
		w.removeAtLocation(ent, loc)
	}
}

func (w *world) removeAtLocation(ent Entity, loc *storage.Location) {
	archIndex := loc.Archetype
	componentIndex := loc.Component
	archetype := w.archetypes[archIndex]
	archetype.SwapRemove(int(componentIndex))
	w.components.Remove(archetype, componentIndex)
	if int(componentIndex) < len(archetype.Entities()) {
		swapped := archetype.Entities()[componentIndex]
		w.entities.Set(swapped.ID(), loc)
	}
	w.destroyed = append(w.destroyed, ent.IncVersion())
}

func (w *world) TransferArchetype(from, to storage.ArchetypeIndex, idx storage.ComponentIndex) storage.ComponentIndex {
	if from == to {
		return idx
	}
	fromArch := w.archetypes[from]
	toArch := w.archetypes[to]

	// move entity id
	ent := fromArch.SwapRemove(int(idx))
	toArch.PushEntity(ent)
	w.entities.Insert(ent.ID(), to, storage.ComponentIndex(len(toArch.Entities())-1))

	if len(fromArch.Entities()) > int(idx) {
		moved := fromArch.Entities()[idx]
		w.entities.Insert(moved.ID(), from, idx)
	}

	// creates component if not exists in new layout
	fromLayout := fromArch.Layout()
	toLayout := toArch.Layout()
	for _, componentType := range toLayout.Components() {
		if !fromLayout.HasComponent(componentType) {
			store := w.components.Storage(componentType)
			store.PushComponent(componentType, to)
		}
	}

	// move components
	for _, componentType := range fromLayout.Components() {
		store := w.components.Storage(componentType)
		if toLayout.HasComponent(componentType) {
			store.MoveComponent(from, idx, to)
		} else {
			store.SwapRemove(from, idx)
		}
	}
	w.components.Move(from, to)

	return storage.ComponentIndex(len(toArch.Entities()) - 1)
}

func (w *world) StorageAccessor() StorageAccessor {
	return StorageAccessor{
		w.index,
		w.components,
		w.archetypes,
	}
}

func (w *world) Archetypes() []*storage.Archetype {
	return w.archetypes
}

func (w *world) nextEntity() Entity {
	if len(w.destroyed) == 0 {
		id := w.nextEntityId
		w.nextEntityId++
		return entity.NewEntity(id)
	}
	newEntity := w.destroyed[len(w.destroyed)-1]
	w.destroyed = w.destroyed[:len(w.destroyed)-1]
	return newEntity
}

func (w *world) insertArchetype(layout *storage.Layout) storage.ArchetypeIndex {
	w.index.Push(layout)
	archIndex := storage.ArchetypeIndex(len(w.archetypes))
	w.archetypes = append(w.archetypes, storage.NewArchetype(archIndex, layout))

	return archIndex
}

func (w *world) getArchetypeForComponents(components []component.IComponentType) storage.ArchetypeIndex {
	if len(components) == 0 {
		panic("entity must have at least one component")
	}
	if ii := w.index.Search(filter.Exact(components)); ii.HasNext() {
		return ii.Next()
	}
	if !w.noDuplicates(components) {
		panic(fmt.Sprintf("duplicate component types: %v", components))
	}
	return w.insertArchetype(storage.NewLayout(components))
}

func (w *world) noDuplicates(components []component.IComponentType) bool {
	// check if there are duplicate values inside components slice
	for i := 0; i < len(components); i++ {
		for j := i + 1; j < len(components); j++ {
			if components[i] == components[j] {
				return false
			}
		}
	}
	return true
}
