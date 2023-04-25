package ecs

import (
	"fmt"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/entity"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
)

// WorldId is a unique identifier for a world.
type WorldId int

// World is a collection of entityLocationStore and componentStore.
type World interface {
	// ID returns the unique identifier for the world.
	ID() WorldId
	// Create creates a new entity with the specified componentStore.
	Create(components ...component.IComponentType) (storage.Entity, error)
	// CreateMany creates a new entity with the specified componentStore.
	CreateMany(n int, components ...component.IComponentType) ([]storage.Entity, error)
	// Entry returns an entry for the specified entity.
	Entry(storage.Entity) (*storage.Entry, error)
	// Remove removes the specified entity.
	Remove(storage.Entity) error
	// Valid returns true if the specified entity is valid.
	Valid(storage.Entity) (bool, error)
	// Len returns the number of entities in the world.
	Len() (int, error)
	// StorageAccessor returns an accessor for the world's storage.
	// It is used to access componentStore and archetypeStore by queries.
	StorageAccessor() StorageAccessor
	// Update loops through and executes all the systems in the world
	Update()
	// AddSystem adds a system to the world.
	AddSystem(System)
	// RegisterComponents registers the components in the world.
	RegisterComponents(...IComponentType)
}

// StorageAccessor is an accessor for the world's storage.
type StorageAccessor struct {
	// Index is the search archCompIndexStore for the world.
	Index storage.ArchetypeComponentIndex
	// Components is the component storage for the world.
	Components *storage.Components
	// Archetypes is the archetype storage for the world.
	Archetypes storage.ArchetypeAccessor
}

type initializer func(w World)

var _ World = &world{}

type world struct {
	id      WorldId
	store   storage.WorldStorage
	systems []System
}

func (w *world) SetEntryLocation(id entity.ID, location storage.Location) error {
	err := w.store.EntryStore.SetLocation(id, location)
	if err != nil {
		return err
	}
	return nil
}

func (w *world) Component(componentType component.IComponentType, index storage.ArchetypeIndex, componentIndex storage.ComponentIndex) ([]byte, error) {
	return w.store.CompStore.Storage(componentType).Component(index, componentIndex)
}

func (w *world) SetComponent(cType component.IComponentType, component []byte, index storage.ArchetypeIndex, componentIndex storage.ComponentIndex) error {
	return w.store.CompStore.Storage(cType).SetComponent(index, componentIndex, component)
}

func (w *world) GetLayout(index storage.ArchetypeIndex) []component.IComponentType {
	return w.store.ArchAccessor.Archetype(index).Layout().Components()
}

func (w *world) GetArchetypeForComponents(componentTypes []component.IComponentType) storage.ArchetypeIndex {
	return w.getArchetypeForComponents(componentTypes)
}

func (w *world) Archetype(index storage.ArchetypeIndex) storage.ArchetypeStorage {
	return w.store.ArchAccessor.Archetype(index)
}

func (w *world) AddSystem(s System) {
	w.systems = append(w.systems, s)
}

func (w *world) Update() {
	for _, sys := range w.systems {
		sys(w)
	}
}

func (w *world) RegisterComponents(ct ...IComponentType) {
	type Initializer interface {
		Initialize(storage.WorldAccessor)
	}
	for _, c := range ct {
		compInitializer, ok := c.(Initializer)
		if !ok {
			panic("cannot initialize component.")
		}
		compInitializer.Initialize(w)
	}
}

var nextWorldId WorldId = 0

var registeredInitializers []initializer

// RegisterInitializer registers an initializer for a world.
func RegisterInitializer(initializer initializer) {
	registeredInitializers = append(registeredInitializers, initializer)
}

// NewWorld creates a new world.
func NewWorld(s storage.WorldStorage) World {
	// TODO: this should prob be handled by redis as well...
	worldId := nextWorldId
	nextWorldId++
	w := &world{
		id:      worldId,
		store:   s,
		systems: make([]System, 0, 256), // this can just stay in memory.
	}
	for _, initializer := range registeredInitializers {
		initializer(w)
	}
	return w
}

func (w *world) ID() WorldId {
	return w.id
}

func (w *world) CreateMany(num int, components ...component.IComponentType) ([]storage.Entity, error) {
	archetypeIndex := w.getArchetypeForComponents(components)
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
	archetypeIndex := w.getArchetypeForComponents(components)
	return w.createEntity(archetypeIndex)
}

func (w *world) createEntity(archetypeIndex storage.ArchetypeIndex) (storage.Entity, error) {
	nextEntity, err := w.nextEntity()
	if err != nil {
		return 0, err
	}
	archetype := w.store.ArchAccessor.Archetype(archetypeIndex)
	componentIndex, err := w.store.CompStore.PushComponents(archetype.Layout().Components(), archetypeIndex)
	if err != nil {
		return 0, err
	}
	err = w.store.EntityLocStore.Insert(nextEntity.ID(), archetypeIndex, componentIndex)
	if err != nil {
		return 0, err
	}
	archetype.PushEntity(nextEntity)
	err = w.createEntry(nextEntity)
	return nextEntity, err
}

func (w *world) createEntry(e storage.Entity) error {
	id := e.ID()
	loc, err := w.store.EntityLocStore.Location(id)
	if err != nil {
		return err
	}
	entry := storage.NewEntry(id, loc)
	return w.store.EntryStore.SetEntry(id, entry)
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
	a := loc.ArchIndex
	c := loc.CompIndex
	// If the version of the entity is not the same as the version of the archetype,
	// the entity is invalid (it means the entity is already destroyed).
	return loc.Valid && e == w.store.ArchAccessor.Archetype(a).Entities()[c], nil
}

func (w *world) Entry(entity storage.Entity) (*storage.Entry, error) {
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
	err = w.store.EntryStore.SetLocation(id, *loc)
	if err != nil {
		return nil, err
	}
	return entry, nil
}

func (w *world) Len() (int, error) {
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

func (w *world) removeAtLocation(ent storage.Entity, loc *storage.Location) error {
	archIndex := loc.ArchIndex
	componentIndex := loc.CompIndex
	archetype := w.store.ArchAccessor.Archetype(archIndex)
	archetype.SwapRemove(int(componentIndex))
	err := w.store.CompStore.Remove(archIndex, archetype.Layout().Components(), componentIndex)
	if err != nil {
		return err
	}
	if int(componentIndex) < len(archetype.Entities()) {
		swapped := archetype.Entities()[componentIndex]
		if err := w.store.EntityLocStore.Set(swapped.ID(), loc); err != nil {
			return err
		}
	}
	w.store.EntityMgr.Destroy(ent.IncVersion())
	return nil
}

func (w *world) TransferArchetype(from storage.ArchetypeIndex, to storage.ArchetypeIndex, idx storage.ComponentIndex) (storage.ComponentIndex, error) {
	if from == to {
		return idx, nil
	}
	fromArch := w.store.ArchAccessor.Archetype(from)
	toArch := w.store.ArchAccessor.Archetype(to)

	// move entity id
	ent := fromArch.SwapRemove(int(idx))
	toArch.PushEntity(ent)
	err := w.store.EntityLocStore.Insert(ent.ID(), to, storage.ComponentIndex(len(toArch.Entities())-1))
	if err != nil {
		return 0, err
	}

	if len(fromArch.Entities()) > int(idx) {
		moved := fromArch.Entities()[idx]
		err := w.store.EntityLocStore.Insert(moved.ID(), from, idx)
		if err != nil {
			return 0, err
		}
	}

	// creates component if not exists in new layout
	fromLayout := fromArch.Layout()
	toLayout := toArch.Layout()
	for _, componentType := range toLayout.Components() {
		if !fromLayout.HasComponent(componentType) {
			store := w.store.CompStore.Storage(componentType)
			if err := store.PushComponent(componentType, to); err != nil {
				return 0, err
			}
		}
	}

	// move componentStore
	for _, componentType := range fromLayout.Components() {
		store := w.store.CompStore.Storage(componentType)
		if toLayout.HasComponent(componentType) {
			if err := store.MoveComponent(from, idx, to); err != nil {
				return 0, err
			}
		} else {
			_, err := store.SwapRemove(from, idx)
			if err != nil {
				return 0, err
			}
		}
	}
	err = w.store.CompStore.Move(from, to)
	if err != nil {
		return 0, err
	}

	return storage.ComponentIndex(len(toArch.Entities()) - 1), nil
}

func (w *world) StorageAccessor() StorageAccessor {
	return StorageAccessor{
		w.store.ArchCompIdxStore,
		&w.store.CompStore,
		w.store.ArchAccessor,
	}
}

func (w *world) nextEntity() (storage.Entity, error) {
	return w.store.EntityMgr.NewEntity()
}

func (w *world) insertArchetype(layout *storage.Layout) storage.ArchetypeIndex {
	w.store.ArchCompIdxStore.Push(layout)
	archIndex := storage.ArchetypeIndex(w.store.ArchAccessor.Count())

	w.store.ArchAccessor.PushArchetype(archIndex, layout)
	return archIndex
}

func (w *world) getArchetypeForComponents(components []component.IComponentType) storage.ArchetypeIndex {
	if len(components) == 0 {
		panic("entity must have at least one component")
	}
	if ii := w.store.ArchCompIdxStore.Search(filter.Exact(components)); ii.HasNext() {
		return ii.Next()
	}
	if !w.noDuplicates(components) {
		panic(fmt.Sprintf("duplicate component types: %v", components))
	}
	return w.insertArchetype(storage.NewLayout(components))
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
