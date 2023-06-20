package ecs

import (
	"errors"
	"fmt"
	"sync"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	"github.com/argus-labs/world-engine/cardinal/ecs/transaction"
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
	id                       WorldId
	store                    storage.WorldStorage
	systems                  []System
	tick                     int
	registeredComponents     []IComponentType
	registeredTransactions   []transaction.ITransaction
	isComponentsRegistered   bool
	isTransactionsRegistered bool
	stateIsLoaded            bool
	// txQueues is a map of transaction names to the relevant list of transactions data
	txQueues map[transaction.TypeID][]any
	// txLock ensures txQueues is not modified in the middle of a tick.
	txLock sync.Mutex

	errs []error
}

var (
	ErrorComponentRegistrationMustHappenOnce   = errors.New("component registration must happen exactly 1 time")
	ErrorTransactionRegistrationMustHappenOnce = errors.New("transaction registration must happen exactly 1 time")
	ErrorStoreStateInvalid                     = errors.New("saved world state is not valid")
)

func (w *World) SetEntityLocation(id storage.EntityID, location storage.Location) error {
	err := w.store.EntityLocStore.SetLocation(id, location)
	if err != nil {
		return err
	}
	return nil
}

func (w *World) Component(componentType component.IComponentType, archID storage.ArchetypeID, componentIndex storage.ComponentIndex) ([]byte, error) {
	return w.store.CompStore.Storage(componentType).Component(archID, componentIndex)
}

func (w *World) SetComponent(cType component.IComponentType, component []byte, archID storage.ArchetypeID, componentIndex storage.ComponentIndex) error {
	return w.store.CompStore.Storage(cType).SetComponent(archID, componentIndex, component)
}

func (w *World) GetLayout(archID storage.ArchetypeID) []component.IComponentType {
	return w.store.ArchAccessor.Archetype(archID).Layout().Components()
}

func (w *World) GetArchetypeForComponents(componentTypes []component.IComponentType) storage.ArchetypeID {
	return w.getArchetypeForComponents(componentTypes)
}

func (w *World) Archetype(archID storage.ArchetypeID) storage.ArchetypeStorage {
	return w.store.ArchAccessor.Archetype(archID)
}

func (w *World) AddSystem(s System) {
	if w.stateIsLoaded {
		panic("cannot register systems after loading game state")
	}
	w.systems = append(w.systems, s)
}

// RegisterComponents attempts to initialize the given slice of components with a WorldAccessor.
// This will give components the ability to access their own data.
func (w *World) RegisterComponents(components ...component.IComponentType) error {
	if w.stateIsLoaded {
		panic("cannot register components after loading game state")
	}
	if w.isComponentsRegistered {
		return ErrorComponentRegistrationMustHappenOnce
	}
	w.isComponentsRegistered = true
	w.registeredComponents = components

	for i, c := range components {
		id := component.TypeID(i + 1)
		if err := c.SetID(id); err != nil {
			return err
		}
	}
	return nil
}

func (w *World) RegisterTransactions(txs ...transaction.ITransaction) error {
	if w.stateIsLoaded {
		panic("cannot register transactions after loading game state")
	}
	if w.isTransactionsRegistered {
		return ErrorTransactionRegistrationMustHappenOnce
	}
	w.isTransactionsRegistered = true
	w.registeredTransactions = txs

	for i, t := range txs {
		id := transaction.TypeID(i + 1)
		if err := t.SetID(id); err != nil {
			return err
		}
	}
	return nil
}

var nextWorldId WorldId = 0

// NewWorld creates a new world.
func NewWorld(s storage.WorldStorage) (*World, error) {
	// TODO: this should prob be handled by redis as well...
	worldId := nextWorldId
	nextWorldId++
	w := &World{
		id:       worldId,
		store:    s,
		tick:     0,
		systems:  make([]System, 0, 256), // this can just stay in memory.
		txQueues: map[transaction.TypeID][]any{},
	}
	return w, nil
}

func (w *World) ID() WorldId {
	return w.id
}

func (w *World) CurrentTick() int {
	return w.tick
}

func (w *World) CreateMany(num int, components ...component.IComponentType) ([]storage.EntityID, error) {
	archetypeID := w.getArchetypeForComponents(components)
	entities := make([]storage.EntityID, 0, num)
	for i := 0; i < num; i++ {
		e, err := w.createEntity(archetypeID)
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

func (w *World) createEntity(archetypeID storage.ArchetypeID) (storage.EntityID, error) {
	nextEntityID, err := w.nextEntity()
	if err != nil {
		return 0, err
	}
	archetype := w.store.ArchAccessor.Archetype(archetypeID)
	componentIndex, err := w.store.CompStore.PushComponents(archetype.Layout().Components(), archetypeID)
	if err != nil {
		return 0, err
	}
	err = w.store.EntityLocStore.Insert(nextEntityID, archetypeID, componentIndex)
	if err != nil {
		return 0, err
	}
	archetype.PushEntity(nextEntityID)

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
	loc, err := w.store.EntityLocStore.GetLocation(id)
	if err != nil {
		return false, err
	}
	a := loc.ArchID
	c := loc.CompIndex
	// If the version of the entity is not the same as the version of the archetype,
	// the entity is invalid (it means the entity is already destroyed).
	return id == w.store.ArchAccessor.Archetype(a).Entities()[c], nil
}

// Entity converts an EntityID to an Entity. An Entity has storage specific details
// about where data for this entity is located
func (w *World) Entity(id storage.EntityID) (storage.Entity, error) {
	loc, err := w.store.EntityLocStore.GetLocation(id)
	if err != nil {
		return storage.BadEntity, err
	}
	return storage.NewEntity(id, loc), nil
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
		loc, err := w.store.EntityLocStore.GetLocation(id)
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
	archID := loc.ArchID
	componentIndex := loc.CompIndex
	archetype := w.store.ArchAccessor.Archetype(archID)
	archetype.SwapRemove(componentIndex)
	err := w.store.CompStore.Remove(archID, archetype.Layout().Components(), componentIndex)
	if err != nil {
		return err
	}
	if int(componentIndex) < len(archetype.Entities()) {
		swappedID := archetype.Entities()[componentIndex]
		if err := w.store.EntityLocStore.SetLocation(swappedID, loc); err != nil {
			return err
		}
	}
	w.store.EntityMgr.Destroy(id)
	return nil
}

func (w *World) TransferArchetype(from storage.ArchetypeID, to storage.ArchetypeID, idx storage.ComponentIndex) (storage.ComponentIndex, error) {
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
func (w *World) copyTransactions() map[transaction.TypeID][]any {
	w.txLock.Lock()
	defer w.txLock.Unlock()
	txsMap := make(map[transaction.TypeID][]any, len(w.txQueues))
	for id, txs := range w.txQueues {
		txsMap[id] = make([]interface{}, len(txs))
		copy(txsMap[id], txs)
		w.txQueues[id] = w.txQueues[id][:0]
	}
	return txsMap
}

// addTransaction adds a transaction to the transaction queue. This should not be used directly.
// Instead, use a TransactionType.AddToQueue to ensure type consistency.
func (w *World) addTransaction(id transaction.TypeID, v any) {
	w.txLock.Lock()
	defer w.txLock.Unlock()
	w.txQueues[id] = append(w.txQueues[id], v)
}

// Tick performs one game tick. This consists of taking a snapshot of all pending transactions, then calling
// each System in turn with the snapshot of transactions.
func (w *World) Tick() error {
	if !w.stateIsLoaded {
		return errors.New("must load state before first tick")
	}
	txs := w.copyTransactions()

	if err := w.store.TickStore.StartNextTick(w.registeredTransactions, txs); err != nil {
		return err
	}

	txQueue := &TransactionQueue{
		queue: txs,
	}

	for _, sys := range w.systems {
		if err := sys(w, txQueue); err != nil {
			return err
		}
	}

	if err := w.saveArchetypeData(); err != nil {
		return err
	}

	if err := w.store.TickStore.FinalizeTick(); err != nil {
		return err
	}
	w.tick++
	return nil
}

const (
	storeArchetypeCompIdxKey  = "arch_component_index"
	storeArchetypeAccessorKey = "arch_accessor"
)

func (w *World) saveArchetypeData() error {
	if err := w.saveToKey(storeArchetypeAccessorKey, w.store.ArchAccessor); err != nil {
		return err
	}
	if err := w.saveToKey(storeArchetypeCompIdxKey, w.store.ArchCompIdxStore); err != nil {
		return err
	}
	return nil
}

func (w *World) saveToKey(key string, cm storage.ComponentMarshaler) error {
	buf, err := cm.Marshal()
	if err != nil {
		return err
	}
	return w.store.StateStore.Save(key, buf)
}

// recoverGameState checks the status of the last game tick. If the tick was incomplete (indicating
// a problem when running one of the Systems), the snapshotted state is recovered and the pending
// transactions for the incomplete tick are returned. A nil recoveredTxs indicates there are no pending
// transactions that need to be processed because the last tick was successful.
func (w *World) recoverGameState() (recoveredTxs map[transaction.TypeID][]any, err error) {
	start, end, err := w.store.TickStore.GetTickNumbers()
	if err != nil {
		return nil, err
	}
	w.tick = end
	// We successfully completed the last tick. Everything is fine
	if start == end {
		return nil, nil
	}
	return w.store.TickStore.Recover(w.registeredTransactions)
}

func (w *World) LoadGameState() error {
	if w.stateIsLoaded {
		return errors.New("cannot load game state multiple times")
	}
	w.stateIsLoaded = true
	recoveredTxs, err := w.recoverGameState()
	if err != nil {
		return err
	}

	if err := w.loadFromKey(storeArchetypeAccessorKey, w.store.ArchAccessor, w.registeredComponents); err != nil {
		return err
	}
	if err := w.loadFromKey(storeArchetypeCompIdxKey, w.store.ArchCompIdxStore, w.registeredComponents); err != nil {
		return err
	}

	if recoveredTxs != nil {
		w.txQueues = recoveredTxs
		if err := w.Tick(); err != nil {
			return err
		}
	}

	return nil
}

func (w *World) loadFromKey(key string, cm storage.ComponentMarshaler, comps []IComponentType) error {
	buf, ok, err := w.store.StateStore.Load(key)
	if !ok {
		// There is no saved data for this key
		return nil
	} else if err != nil {
		return err
	}
	return cm.UnmarshalWithComps(buf, comps)
}

func (w *World) GetComponentsOnEntity(id storage.EntityID) ([]IComponentType, error) {
	ent, err := w.Entity(id)
	if err != nil {
		return nil, err
	}
	return w.GetLayout(ent.Loc.ArchID), nil
}

func (w *World) nextEntity() (storage.EntityID, error) {
	return w.store.EntityMgr.NewEntity()
}

func (w *World) insertArchetype(layout *storage.Layout) storage.ArchetypeID {
	w.store.ArchCompIdxStore.Push(layout)
	archID := storage.ArchetypeID(w.store.ArchAccessor.Count())

	w.store.ArchAccessor.PushArchetype(archID, layout)
	return archID
}

func (w *World) getArchetypeForComponents(components []component.IComponentType) storage.ArchetypeID {
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

func (w *World) LogError(err error) {
	w.errs = append(w.errs, err)
}
