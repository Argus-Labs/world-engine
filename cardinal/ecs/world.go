package ecs

import (
	"fmt"
	"sync"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
)

// WorldId is a unique identifier for a world.
type WorldId int

// StorageAccessor is an accessor for the world's storage.
type StorageAccessor struct {
	// Index is the search archCompIndexStore for the world.
	Index storage.ArchetypeComponentIndex
	// Components is the component storage for the world.
	Components *storage.Components
	// Archetypes is the archetype storage for the world.
	Archetypes storage.ArchetypeAccessor
}

type World struct {
	id      WorldId
	store   storage.WorldStorage
	systems []System
	// txNames contains the set of transaction names that have been registered for this world. It is an error
	// to attempt to register the same transaction name more than once.
	txNames map[string]bool
	// txQueues is a map of transaction names to the relevant list of transactions data
	txQueues map[string][]interface{}
	// txLock ensures txQueues is not modified in the middle of a tick.
	txLock sync.Mutex
}

// AddTxName adds the given transaction name to the set of all transaction names seen so far. If the transaction
// name has already been added, false is returned.
func (w *World) AddTxName(name string) (ok bool) {
	if w.txNames[name] {
		return false
	}
	w.txNames[name] = true
	return true
}

func (w *World) SetEntityLocation(id storage.EntityID, location storage.Location) error {
	err := w.store.EntityStore.SetLocation(id, location)
	if err != nil {
		return err
	}
	return nil
}

func (w *World) Component(componentType component.IComponentType, index storage.ArchetypeIndex, componentIndex storage.ComponentIndex) ([]byte, error) {
	return w.store.CompStore.Storage(componentType).Component(index, componentIndex)
}

func (w *World) SetComponent(cType component.IComponentType, component []byte, index storage.ArchetypeIndex, componentIndex storage.ComponentIndex) error {
	return w.store.CompStore.Storage(cType).SetComponent(index, componentIndex, component)
}

func (w *World) GetLayout(index storage.ArchetypeIndex) []component.IComponentType {
	return w.store.ArchAccessor.Archetype(index).Layout().Components()
}

func (w *World) GetArchetypeForComponents(componentTypes []component.IComponentType) storage.ArchetypeIndex {
	return w.getArchetypeForComponents(componentTypes)
}

func (w *World) Archetype(index storage.ArchetypeIndex) storage.ArchetypeStorage {
	return w.store.ArchAccessor.Archetype(index)
}

func (w *World) AddSystem(s System) {
	w.systems = append(w.systems, s)
}

type Initializer interface {
	Initialize(world *World) error
}

// RegisterComponents attempts to initialize the given slice of components with a WorldAccessor.
// This will give components the ability to access their own data.
func (w *World) RegisterComponents(inits ...Initializer) {
	for _, in := range inits {
		if err := in.Initialize(w); err != nil {
			panic(fmt.Sprintf("cannot initialize component: %v", err))
		}
	}
}

var nextWorldId WorldId = 0

// NewWorld creates a new world.
func NewWorld(s storage.WorldStorage) *World {
	// TODO: this should prob be handled by redis as well...
	worldId := nextWorldId
	nextWorldId++
	w := &World{
		id:       worldId,
		store:    s,
		systems:  make([]System, 0, 256), // this can just stay in memory.
		txNames:  map[string]bool{},
		txQueues: map[string][]interface{}{},
	}
	return w
}

func (w *World) ID() WorldId {
	return w.id
}

func (w *World) CreateMany(num int, components ...component.IComponentType) ([]storage.EntityID, error) {
	archetypeIndex := w.getArchetypeForComponents(components)
	entities := make([]storage.EntityID, 0, num)
	for i := 0; i < num; i++ {
		e, err := w.createEntity(archetypeIndex)
		if err != nil {
			return nil, err
		}

		entities = append(entities, e)
	}
	return entities, nil
}

func (w *World) Create(components ...component.IComponentType) (storage.EntityID, error) {
	entities, err := w.CreateMany(1, components...)
	if err != nil {
		return 0, err
	}
	return entities[0], nil
}

func (w *World) createEntity(archetypeIndex storage.ArchetypeIndex) (storage.EntityID, error) {
	nextEntityID, err := w.nextEntity()
	if err != nil {
		return 0, err
	}
	archetype := w.store.ArchAccessor.Archetype(archetypeIndex)
	componentIndex, err := w.store.CompStore.PushComponents(archetype.Layout().Components(), archetypeIndex)
	if err != nil {
		return 0, err
	}
	err = w.store.EntityLocStore.Insert(nextEntityID, archetypeIndex, componentIndex)
	if err != nil {
		return 0, err
	}
	archetype.PushEntity(nextEntityID)

	loc, err := w.store.EntityLocStore.Location(nextEntityID)
	if err != nil {
		return 0, err
	}
	entity := storage.NewEntity(nextEntityID, loc)
	if err := w.store.EntityStore.SetEntity(nextEntityID, entity); err != nil {
		return 0, err
	}

	return nextEntityID, err
}

func (w *World) Valid(id storage.EntityID) (bool, error) {
	if id == storage.BadID {
		return false, nil
	}
	ok, err := w.store.EntityLocStore.ContainsEntity(id)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	loc, err := w.store.EntityLocStore.Location(id)
	if err != nil {
		return false, err
	}
	a := loc.ArchIndex
	c := loc.CompIndex
	// If the version of the entity is not the same as the version of the archetype,
	// the entity is invalid (it means the entity is already destroyed).
	return id == w.store.ArchAccessor.Archetype(a).Entities()[c], nil
}

// Entity converts an EntityID to an Entity. An Entity has storage specific details
// about where data for this entity is located
func (w *World) Entity(id storage.EntityID) (storage.Entity, error) {
	entity, err := w.store.EntityStore.GetEntity(id)
	if err != nil {
		return storage.BadEntity, err
	}
	loc, err := w.store.EntityLocStore.Location(id)
	if err != nil {
		return storage.BadEntity, err
	}
	err = w.store.EntityStore.SetLocation(id, loc)
	if err != nil {
		return storage.BadEntity, err
	}
	return entity, nil
}

// Len return the number of entities in this world
func (w *World) Len() (int, error) {
	l, err := w.store.EntityLocStore.Len()
	if err != nil {
		return 0, err
	}
	return l, nil
}

// Remove removes the given Entity from the world
func (w *World) Remove(id storage.EntityID) error {
	ok, err := w.Valid(id)
	if err != nil {
		return err
	}
	if ok {
		loc, err := w.store.EntityLocStore.Location(id)
		if err != nil {
			return err
		}
		if err := w.store.EntityLocStore.Remove(id); err != nil {
			return err
		}
		if err := w.removeAtLocation(id, loc); err != nil {
			return err
		}
	}
	return nil
}

func (w *World) removeAtLocation(id storage.EntityID, loc storage.Location) error {
	archIndex := loc.ArchIndex
	componentIndex := loc.CompIndex
	archetype := w.store.ArchAccessor.Archetype(archIndex)
	archetype.SwapRemove(componentIndex)
	err := w.store.CompStore.Remove(archIndex, archetype.Layout().Components(), componentIndex)
	if err != nil {
		return err
	}
	if int(componentIndex) < len(archetype.Entities()) {
		swappedID := archetype.Entities()[componentIndex]
		if err := w.store.EntityLocStore.Set(swappedID, loc); err != nil {
			return err
		}
		if err := w.store.EntityStore.SetLocation(swappedID, loc); err != nil {
			return err
		}
	}
	w.store.EntityMgr.Destroy(id)
	return nil
}

func (w *World) TransferArchetype(from storage.ArchetypeIndex, to storage.ArchetypeIndex, idx storage.ComponentIndex) (storage.ComponentIndex, error) {
	if from == to {
		return idx, nil
	}
	fromArch := w.store.ArchAccessor.Archetype(from)
	toArch := w.store.ArchAccessor.Archetype(to)

	// move entity id
	id := fromArch.SwapRemove(idx)
	toArch.PushEntity(id)
	err := w.store.EntityLocStore.Insert(id, to, storage.ComponentIndex(len(toArch.Entities())-1))
	if err != nil {
		return 0, err
	}

	if len(fromArch.Entities()) > int(idx) {
		movedID := fromArch.Entities()[idx]
		err := w.store.EntityLocStore.Insert(movedID, from, idx)
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

func (w *World) StorageAccessor() StorageAccessor {
	return StorageAccessor{
		w.store.ArchCompIdxStore,
		&w.store.CompStore,
		w.store.ArchAccessor,
	}
}

// copyTransactions makes a copy of the world txQueue, then zeroes out the txQueue.
func (w *World) copyTransactions() map[string][]interface{} {
	w.txLock.Lock()
	defer w.txLock.Unlock()
	txsMap := make(map[string][]interface{}, len(w.txQueues))
	for name, txs := range w.txQueues {
		txsMap[name] = make([]interface{}, len(txs))
		copy(txsMap[name], txs)
		w.txQueues[name] = w.txQueues[name][:0]
	}
	return txsMap
}

// AddTransaction adds a transaction to the transaction queue. This should not be used directly.
// Instead, use a TransactionType.AddToQueue to ensure type consistency.
func (w *World) AddTransaction(name string, v any) {
	w.txLock.Lock()
	defer w.txLock.Unlock()
	w.txQueues[name] = append(w.txQueues[name], v)
}

// Tick performs one game tick. This consists of taking a snapshot of all pending transactions, then calling
// each System in turn with the snapshot of transactions.
func (w *World) Tick() {
	txs := w.copyTransactions()
	txQueue := &TransactionQueue{
		queue: txs,
	}

	for _, sys := range w.systems {
		sys(txQueue)
	}
}

func (w *World) GetComponentsOnEntity(id storage.EntityID) ([]IComponentType, error) {
	ent, err := w.Entity(id)
	if err != nil {
		return nil, err
	}
	return w.GetLayout(ent.Loc.ArchIndex), nil
}

func (w *World) nextEntity() (storage.EntityID, error) {
	return w.store.EntityMgr.NewEntity()
}

func (w *World) insertArchetype(layout *storage.Layout) storage.ArchetypeIndex {
	w.store.ArchCompIdxStore.Push(layout)
	archIndex := storage.ArchetypeIndex(w.store.ArchAccessor.Count())

	w.store.ArchAccessor.PushArchetype(archIndex, layout)
	return archIndex
}

func (w *World) getArchetypeForComponents(components []component.IComponentType) storage.ArchetypeIndex {
	if len(components) == 0 {
		panic("entity must have at least one component")
	}
	if ii := w.store.ArchCompIdxStore.Search(filter.Exact(components...)); ii.HasNext() {
		return ii.Next()
	}
	if !w.noDuplicates(components) {
		panic(fmt.Sprintf("duplicate component types: %v", components))
	}
	return w.insertArchetype(storage.NewLayout(components))
}

func (w *World) noDuplicates(components []component.IComponentType) bool {
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
