package ecs

import (
	"fmt"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/entity"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	types "github.com/argus-labs/world-engine/cardinal/ecs/storage/types/v1"
)

// WorldId is a unique identifier for a world.
type WorldId int

type comp interface {
	// GetComponent gets the component from the entry.
	GetComponent(*types.Entry, string) (component.IComponentType, error)
	// SetComponent updates the component data for the entry.
	SetComponent(*types.Entry, component.IComponentType) error
}

// World is a collection of entities and components.
type World interface {
	comp
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
	RegisterComponents(...component.IComponentType)
}

type initializer func(w World)

var _ World = &world{}

type world struct {
	id      WorldId
	store   storage.WorldStorage
	systems []System
	tr      storage.TypeRegistry
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
		Storage(componentType).
		SetComponent(storage.ArchetypeIndex(ai), storage.ComponentIndex(ci), comp)
	return err
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
	for _, initializer := range registeredInitializers {
		initializer(w)
	}
	return w
}

func (w *world) SetEntryLocation(id entity.ID, location *types.Location) error {
	err := w.store.EntryStore.SetLocation(id, location)
	if err != nil {
		return err
	}
	return nil
}

func (w *world) Component(componentType component.IComponentType, index storage.ArchetypeIndex, componentIndex storage.ComponentIndex) (component.IComponentType, error) {
	comp, err := w.store.CompStore.Storage(componentType).Component(index, componentIndex)
	return comp, err

}

func (w *world) GetLayout(index storage.ArchetypeIndex) ([]component.IComponentType, error) {
	arch, err := w.store.ArchAccessor.Archetype(index)
	if err != nil {
		return nil, err
	}
	anys := arch.GetComponents()
	// TODO(technicallyty): unmarshal any? why do we even need this function..
	_ = anys
	return nil, nil
}

func (w *world) GetArchetypeForComponents(componentTypes []component.IComponentType) (storage.ArchetypeIndex, error) {
	ai, err := w.getArchetypeForComponents(componentTypes)
	return ai, err
}

func (w *world) Archetype(index storage.ArchetypeIndex) (*types.Archetype, error) {
	arch, err := w.store.ArchAccessor.Archetype(index)
	return arch, err
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
	//type Initializer interface {
	//	Initialize(storage.WorldAccessor)
	//}
	//for _, c := range ct {
	//	compInitializer, ok := c.(Initializer)
	//	if !ok {
	//		panic("cannot initialize component.")
	//	}
	//	compInitializer.Initialize(w)
	//}
	for _, c := range ct {
		w.tr.Register(c)
	}
}

var nextWorldId WorldId = 0

var registeredInitializers []initializer

// RegisterInitializer registers an initializer for a world.
func RegisterInitializer(initializer initializer) {
	registeredInitializers = append(registeredInitializers, initializer)
}

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

func (w *world) TransferArchetype(from storage.ArchetypeIndex, to storage.ArchetypeIndex, idx storage.ComponentIndex) (storage.ComponentIndex, error) {
	if from == to {
		return idx, nil
	}
	fromArch, err := w.store.ArchAccessor.Archetype(from)
	if err != nil {
		return 0, err
	}
	toArch, err := w.store.ArchAccessor.Archetype(to)
	if err != nil {
		return 0, err
	}

	// move entity id
	ent, _ := w.store.ArchAccessor.RemoveEntityAt(from, int(idx))
	err = w.store.ArchAccessor.PushEntity(to, ent)
	if err != nil {
		return 0, err
	}
	err = w.store.EntityLocStore.Insert(ent.ID(), to, storage.ComponentIndex(len(toArch.EntityIds)-1))
	if err != nil {
		return 0, err
	}

	if len(fromArch.EntityIds) > int(idx) {
		moved := fromArch.EntityIds[idx]
		err := w.store.EntityLocStore.Insert(entity.ID(moved), from, idx)
		if err != nil {
			return 0, err
		}
	}

	// creates component if not exists in new layout
	fromLayout := fromArch.Components
	toLayout := toArch.Components
	for _, anyComp := range toLayout {
		if !Contains[*anypb.Any](fromLayout, anyComp, func(x, y *anypb.Any) bool {
			return x.TypeUrl == y.TypeUrl
		}) {
			store := w.store.CompStore.StorageFromAny(anyComp)
			if err := store.PushRawComponent(anyComp, to); err != nil {
				return 0, err
			}
		}
	}

	// move component
	for _, anyComp := range fromLayout {
		store := w.store.CompStore.StorageFromAny(anyComp)
		if Contains[*anypb.Any](toLayout, anyComp, func(x, y *anypb.Any) bool {
			return x.TypeUrl == y.TypeUrl
		}) {
			if err := store.MoveComponent(from, idx, to); err != nil {
				return 0, err
			}
		} else {
			err := store.RemoveComponent(from, idx)
			if err != nil {
				return 0, err
			}
		}
	}
	err = w.store.CompStore.Move(from, to)
	if err != nil {
		return 0, err
	}

	return storage.ComponentIndex(len(toArch.EntityIds) - 1), nil
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
