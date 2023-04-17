package ECS

import (
	"fmt"

	"github.com/argus-labs/cardinal/ECS/component"
	"github.com/argus-labs/cardinal/ECS/entity"
	"github.com/argus-labs/cardinal/ECS/filter"
	storage2 "github.com/argus-labs/cardinal/ECS/storage"
)

// WorldId is a unique identifier for a world.
type WorldId int

// World is a collection of entityLocationStore and componentStore.
type World interface {
	// ID returns the unique identifier for the world.
	ID() WorldId
	// Create creates a new entity with the specified componentStore.
	Create(components ...component.IComponentType) (storage2.Entity, error)
	// CreateMany creates a new entity with the specified componentStore.
	CreateMany(n int, components ...component.IComponentType) ([]storage2.Entity, error)
	// Entry returns an entry for the specified entity.
	Entry(entity storage2.Entity) *storage2.Entry
	// Remove removes the specified entity.
	Remove(entity storage2.Entity)
	// Valid returns true if the specified entity is valid.
	Valid(e storage2.Entity) bool
	// Len returns the number of entities in the world.
	Len() int
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
	Index storage2.ArchetypeComponentIndex
	// Components is the component storage for the world.
	Components *storage2.Components
	// Archetypes is the archetype storage for the world.
	Archetypes storage2.ArchetypeAccessor
}

type initializer func(w World)

var _ World = &world{}

type world struct {
	id      WorldId
	store   storage2.WorldStorage
	systems []System
}

func (w *world) SetEntryLocation(id entity.ID, location storage2.Location) {
	w.store.EntryStore.SetLocation(id, location)
}

func (w *world) Component(componentType component.IComponentType, index storage2.ArchetypeIndex, componentIndex storage2.ComponentIndex) []byte {
	return w.store.CompStore.Storage(componentType).Component(index, componentIndex)
}

func (w *world) SetComponent(cType component.IComponentType, component []byte, index storage2.ArchetypeIndex, componentIndex storage2.ComponentIndex) {
	w.store.CompStore.Storage(cType).SetComponent(index, componentIndex, component)
}

func (w *world) GetLayout(index storage2.ArchetypeIndex) []component.IComponentType {
	return w.store.ArchAccessor.Archetype(index).Layout().Components()
}

func (w *world) GetArchetypeForComponents(componentTypes []component.IComponentType) storage2.ArchetypeIndex {
	return w.getArchetypeForComponents(componentTypes)
}

func (w *world) Archetype(index storage2.ArchetypeIndex) storage2.ArchetypeStorage {
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
		Initialize(storage2.WorldAccessor)
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
func NewWorld(s storage2.WorldStorage) World {
	// TODO(technicallyty): this could prob be handled by redis as well...
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

func (w *world) CreateMany(num int, components ...component.IComponentType) ([]storage2.Entity, error) {
	archetypeIndex := w.getArchetypeForComponents(components)
	entities := make([]storage2.Entity, 0, num)
	for i := 0; i < num; i++ {
		e, err := w.createEntity(archetypeIndex)
		if err != nil {
			return nil, err
		}

		entities = append(entities, e)
	}
	return entities, nil
}

func (w *world) Create(components ...component.IComponentType) (storage2.Entity, error) {
	archetypeIndex := w.getArchetypeForComponents(components)
	return w.createEntity(archetypeIndex)
}

func (w *world) createEntity(archetypeIndex storage2.ArchetypeIndex) (storage2.Entity, error) {
	nextEntity := w.nextEntity()
	archetype := w.store.ArchAccessor.Archetype(archetypeIndex)
	componentIndex, err := w.store.CompStore.PushComponents(archetype.Layout().Components(), archetypeIndex)
	if err != nil {
		return 0, err
	}
	w.store.EntityLocStore.Insert(nextEntity.ID(), archetypeIndex, componentIndex)
	archetype.PushEntity(nextEntity)
	w.createEntry(nextEntity)
	return nextEntity, nil
}

func (w *world) createEntry(e storage2.Entity) {
	id := e.ID()
	loc := w.store.EntityLocStore.Location(id)
	entry := storage2.NewEntry(id, e, loc)
	w.store.EntryStore.SetEntry(id, entry)
}

func (w *world) Valid(e storage2.Entity) bool {
	if e == storage2.Null {
		return false
	}
	if !w.store.EntityLocStore.ContainsEntity(e.ID()) {
		return false
	}
	loc := w.store.EntityLocStore.Location(e.ID())
	a := loc.ArchIndex
	c := loc.CompIndex
	// If the version of the entity is not the same as the version of the archetype,
	// the entity is invalid (it means the entity is already destroyed).
	return loc.Valid && e == w.store.ArchAccessor.Archetype(a).Entities()[c]
}

// TODO(technicallyty): we need to update these methods here to not just
// update the struct itself, but update the backend too.
func (w *world) Entry(entity storage2.Entity) *storage2.Entry {
	id := entity.ID()
	entry := w.store.EntryStore.GetEntry(id)
	w.store.EntryStore.SetEntity(id, entity)
	w.store.EntryStore.SetLocation(id, *w.store.EntityLocStore.Location(id))
	return entry
}

func (w *world) Len() int {
	return w.store.EntityLocStore.Len()
}

func (w *world) Remove(ent storage2.Entity) {
	if w.Valid(ent) {
		loc := w.store.EntityLocStore.Location(ent.ID())
		w.store.EntityLocStore.Remove(ent.ID())
		w.removeAtLocation(ent, loc)
	}
}

func (w *world) removeAtLocation(ent storage2.Entity, loc *storage2.Location) {
	archIndex := loc.ArchIndex
	componentIndex := loc.CompIndex
	archetype := w.store.ArchAccessor.Archetype(archIndex)
	archetype.SwapRemove(int(componentIndex))
	w.store.CompStore.Remove(archIndex, archetype.Layout().Components(), componentIndex)
	if int(componentIndex) < len(archetype.Entities()) {
		swapped := archetype.Entities()[componentIndex]
		w.store.EntityLocStore.Set(swapped.ID(), loc)
	}
	w.store.EntityMgr.Destroy(ent.IncVersion())
}

func (w *world) TransferArchetype(from, to storage2.ArchetypeIndex, idx storage2.ComponentIndex) storage2.ComponentIndex {
	if from == to {
		return idx
	}
	fromArch := w.store.ArchAccessor.Archetype(from)
	toArch := w.store.ArchAccessor.Archetype(to)

	// move entity id
	ent := fromArch.SwapRemove(int(idx))
	toArch.PushEntity(ent)
	w.store.EntityLocStore.Insert(ent.ID(), to, storage2.ComponentIndex(len(toArch.Entities())-1))

	if len(fromArch.Entities()) > int(idx) {
		moved := fromArch.Entities()[idx]
		w.store.EntityLocStore.Insert(moved.ID(), from, idx)
	}

	// creates component if not exists in new layout
	fromLayout := fromArch.Layout()
	toLayout := toArch.Layout()
	for _, componentType := range toLayout.Components() {
		if !fromLayout.HasComponent(componentType) {
			store := w.store.CompStore.Storage(componentType)
			// TODO(technicallyty): handle error
			store.PushComponent(componentType, to)
		}
	}

	// move componentStore
	for _, componentType := range fromLayout.Components() {
		store := w.store.CompStore.Storage(componentType)
		if toLayout.HasComponent(componentType) {
			store.MoveComponent(from, idx, to)
		} else {
			store.SwapRemove(from, idx)
		}
	}
	w.store.CompStore.Move(from, to)

	return storage2.ComponentIndex(len(toArch.Entities()) - 1)
}

func (w *world) StorageAccessor() StorageAccessor {
	return StorageAccessor{
		w.store.ArchCompIdxStore,
		&w.store.CompStore,
		w.store.ArchAccessor,
	}
}

func (w *world) nextEntity() storage2.Entity {
	return w.store.EntityMgr.NewEntity()
}

func (w *world) insertArchetype(layout *storage2.Layout) storage2.ArchetypeIndex {
	w.store.ArchCompIdxStore.Push(layout)
	archIndex := storage2.ArchetypeIndex(w.store.ArchAccessor.Count())

	w.store.ArchAccessor.PushArchetype(archIndex, layout)
	return archIndex
}

func (w *world) getArchetypeForComponents(components []component.IComponentType) storage2.ArchetypeIndex {
	if len(components) == 0 {
		panic("entity must have at least one component")
	}
	if ii := w.store.ArchCompIdxStore.Search(filter.Exact(components)); ii.HasNext() {
		return ii.Next()
	}
	if !w.noDuplicates(components) {
		panic(fmt.Sprintf("duplicate component types: %v", components))
	}
	return w.insertArchetype(storage2.NewLayout(components))
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
