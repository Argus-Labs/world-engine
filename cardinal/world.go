package cardinal

import (
	"fmt"

	"github.com/argus-labs/cardinal/component"
	"github.com/argus-labs/cardinal/entity"
	"github.com/argus-labs/cardinal/filter"
	"github.com/argus-labs/cardinal/storage"
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
	Entry(entity storage.Entity) *storage.Entry
	// Remove removes the specified entity.
	Remove(entity storage.Entity)
	// Valid returns true if the specified entity is valid.
	Valid(e storage.Entity) bool
	// Len returns the number of entityLocationStore in the world.
	Len() int
	// StorageAccessor returns an accessor for the world's storage.
	// It is used to access componentStore and archetypeStore by queries.
	StorageAccessor() StorageAccessor
	// Archetypes returns the archetypeStore in the world.
	Archetypes() []*storage.Archetype
	// Update loops through and executes all the systems in the world
	Update()
	// AddSystem adds a system to the world.
	AddSystem(System)
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

type world struct {
	id                  WorldId
	archCompIndexStore  storage.ArchetypeComponentIndex
	entityLocationStore storage.EntityLocationStorage
	componentStore      *storage.Components
	archetypeStore      storage.ArchetypeAccessor
	entityTrashCan      storage.EntityManager
	entryStore          storage.EntryStorage
	systems             []System
}

func (w *world) Component(componentType component.IComponentType, index storage.ArchetypeIndex, componentIndex storage.ComponentIndex) []byte {
	return w.componentStore.Storage(componentType).Component(index, componentIndex)
}

func (w *world) SetComponent(cType component.IComponentType, component []byte, index storage.ArchetypeIndex, componentIndex storage.ComponentIndex) {
	w.componentStore.Storage(cType).SetComponent(index, componentIndex, component)
}

func (w *world) GetLayout(index storage.ArchetypeIndex) []component.IComponentType {
	return w.archetypeStore.Archetype(index).Layout().Components()
}

func (w *world) GetArchetypeForComponents(componentTypes []component.IComponentType) storage.ArchetypeIndex {
	return w.getArchetypeForComponents(componentTypes)
}

func (w *world) Archetype(index storage.ArchetypeIndex) storage.ArchetypeStorage {
	return w.archetypeStore.Archetype(index)
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
		id:                  worldId,
		archCompIndexStore:  storage.NewIndex(),
		entityLocationStore: storage.NewLocationMap(),
		// TODO(technicallyty): update to use dep injection as arguments to NewWorld.
		componentStore: storage.NewComponents(storage.NewComponentsSliceStorage(), storage.NewComponentIndexMap()),
		archetypeStore: storage.NewArchetypeAccessor(),
		entityTrashCan: storage.NewEntityManager(),
		entryStore:     storage.NewEntryStorage(),
		systems:        make([]System, 0, 256), // this can just stay in memory.
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
	nextEntity := w.nextEntity()
	archetype := w.archetypeStore.Archetype(archetypeIndex)
	componentIndex, err := w.componentStore.PushComponents(archetype.Layout().Components(), archetypeIndex)
	if err != nil {
		return 0, err
	}
	w.entityLocationStore.Insert(nextEntity.ID(), archetypeIndex, componentIndex)
	archetype.PushEntity(nextEntity)
	w.createEntry(nextEntity)
	return nextEntity, nil
}

func (w *world) createEntry(e storage.Entity) {
	id := e.ID()
	if int(id) >= w.entryStore.Length() {
		w.entryStore.Push(nil)
	}
	loc := w.entityLocationStore.Location(id)
	entry := storage.NewEntry(id, e, loc, w)
	w.entryStore.Set(id, entry)
}

func (w *world) Valid(e storage.Entity) bool {
	if e == storage.Null {
		return false
	}
	if !w.entityLocationStore.Contains(e.ID()) {
		return false
	}
	loc := w.entityLocationStore.Location(e.ID())
	a := loc.ArchIndex
	c := loc.CompIndex
	// If the version of the entity is not the same as the version of the archetype,
	// the entity is invalid (it means the entity is already entityTrashCan).
	return loc.Valid && e == w.archetypeStore.Archetype(a).Entities()[c]
}

func (w *world) Entry(entity storage.Entity) *storage.Entry {
	id := entity.ID()
	entry := w.entryStore.Get(id)
	entry.SetEntity(entity)
	entry.SetLocation(w.entityLocationStore.Location(id))
	return entry
}

func (w *world) Len() int {
	return w.entityLocationStore.Len()
}

func (w *world) Remove(ent storage.Entity) {
	if w.Valid(ent) {
		loc := w.entityLocationStore.Location(ent.ID())
		w.entityLocationStore.Remove(ent.ID())
		w.removeAtLocation(ent, loc)
	}
}

func (w *world) removeAtLocation(ent storage.Entity, loc *storage.Location) {
	archIndex := loc.ArchIndex
	componentIndex := loc.CompIndex
	archetype := w.archetypeStore.Archetype(archIndex)
	archetype.SwapRemove(int(componentIndex))
	w.componentStore.Remove(archIndex, archetype.Layout().Components(), componentIndex)
	if int(componentIndex) < len(archetype.Entities()) {
		swapped := archetype.Entities()[componentIndex]
		w.entityLocationStore.Set(swapped.ID(), loc)
	}
	w.entityTrashCan.Destroy(ent.IncVersion())
}

func (w *world) TransferArchetype(from, to storage.ArchetypeIndex, idx storage.ComponentIndex) storage.ComponentIndex {
	if from == to {
		return idx
	}
	fromArch := w.archetypeStore.Archetype(from)
	toArch := w.archetypeStore.Archetype(to)

	// move entity id
	ent := fromArch.SwapRemove(int(idx))
	toArch.PushEntity(ent)
	w.entityLocationStore.Insert(ent.ID(), to, storage.ComponentIndex(len(toArch.Entities())-1))

	if len(fromArch.Entities()) > int(idx) {
		moved := fromArch.Entities()[idx]
		w.entityLocationStore.Insert(moved.ID(), from, idx)
	}

	// creates component if not exists in new layout
	fromLayout := fromArch.Layout()
	toLayout := toArch.Layout()
	for _, componentType := range toLayout.Components() {
		if !fromLayout.HasComponent(componentType) {
			store := w.componentStore.Storage(componentType)
			store.PushComponent(componentType, to)
		}
	}

	// move componentStore
	for _, componentType := range fromLayout.Components() {
		store := w.componentStore.Storage(componentType)
		if toLayout.HasComponent(componentType) {
			store.MoveComponent(from, idx, to)
		} else {
			store.SwapRemove(from, idx)
		}
	}
	w.componentStore.Move(from, to)

	return storage.ComponentIndex(len(toArch.Entities()) - 1)
}

func (w *world) StorageAccessor() StorageAccessor {
	return StorageAccessor{
		w.archCompIndexStore,
		w.componentStore,
		w.archetypeStore,
	}
}

func (w *world) Archetypes() []*storage.Archetype {
	return w.archetypeStore.Archetypes()
}

func (w *world) nextEntity() storage.Entity {
	// if the trash is empty, we get the next entity ID, increment, and create a new entity.
	if w.entityTrashCan.Length() == 0 {
		id := w.entityTrashCan.GetNextEntityID()
		return entity.NewEntity(id)
	}
	// entity trash isn't empty, so we can reuse.
	newEntity := w.entityTrashCan.Get(w.entityTrashCan.Length() - 1)
	w.entityTrashCan.Shrink()
	return newEntity
}

func (w *world) insertArchetype(layout *storage.Layout) storage.ArchetypeIndex {
	w.archCompIndexStore.Push(layout)
	archIndex := storage.ArchetypeIndex(w.archetypeStore.Count())

	w.archetypeStore.PushArchetype(archIndex, layout)
	return archIndex
}

func (w *world) getArchetypeForComponents(components []component.IComponentType) storage.ArchetypeIndex {
	if len(components) == 0 {
		panic("entity must have at least one component")
	}
	if ii := w.archCompIndexStore.Search(filter.Exact(components)); ii.HasNext() {
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
