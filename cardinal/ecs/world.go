package ecs

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	shardv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/shard/v1"
	"google.golang.org/protobuf/proto"
	"pkg.world.dev/world-engine/cardinal/ecs/receipt"

	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/chain/x/shard/types"
	"pkg.world.dev/world-engine/sign"

	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/cardinal/shard"
)

// Namespace is a unique identifier for a world.
type Namespace string

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
	namespace                Namespace
	store                    storage.WorldStorage
	systems                  []System
	tick                     uint64
	registeredComponents     []IComponentType
	registeredTransactions   []transaction.ITransaction
	registeredReads          []IRead
	isComponentsRegistered   bool
	isTransactionsRegistered bool
	stateIsLoaded            bool
	// txQueues is a map of transaction names to the relevant list of transactions data
	txQueues transaction.TxMap
	// txLock ensures txQueues is not modified in the middle of a tick.
	txLock sync.Mutex

	receiptHistory *receipt.History

	chain shard.ReadAdapter
	// isRecovering indicates that the world is recovering from the DA layer.
	// this is used to prevent ticks from submitting duplicate transactions the DA layer.
	isRecovering bool

	errs []error
}

var (
	ErrorComponentRegistrationMustHappenOnce   = errors.New("component registration must happen exactly 1 time")
	ErrorTransactionRegistrationMustHappenOnce = errors.New("transaction registration must happen exactly 1 time")
	ErrorStoreStateInvalid                     = errors.New("saved world state is not valid")
	ErrorDuplicateTransactionName              = errors.New("transaction names must be unique")
	ErrorDuplicateReadName                     = errors.New("read names must be unique")
)

func (w *World) IsRecovering() bool {
	return w.isRecovering
}

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
	w.AddSystems(s)
}

func (w *World) AddSystems(s ...System) {
	if w.stateIsLoaded {
		panic("cannot register systems after loading game state")
	}
	w.systems = append(w.systems, s...)
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
	w.registeredComponents = append(w.registeredComponents, SignerComp)
	w.registeredComponents = append(w.registeredComponents, components...)

	for i, c := range w.registeredComponents {
		id := component.TypeID(i + 1)
		if err := c.SetID(id); err != nil {
			return err
		}
	}
	return nil
}

func (w *World) RegisterReads(reads ...IRead) error {
	if w.stateIsLoaded {
		panic("cannot register reads after loading game state")
	}
	w.registeredReads = append(w.registeredReads, reads...)
	seenReadNames := map[string]struct{}{}
	for _, t := range w.registeredReads {
		name := t.Name()
		if _, ok := seenReadNames[name]; ok {
			return fmt.Errorf("duplicate read %q: %w", name, ErrorDuplicateReadName)
		}
		seenReadNames[name] = struct{}{}
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
	w.registerInternalTransactions()
	w.registeredTransactions = append(w.registeredTransactions, txs...)

	seenTxNames := map[string]bool{}
	for i, t := range w.registeredTransactions {
		name := t.Name()
		if seenTxNames[name] {
			return fmt.Errorf("duplicate tx %q: %w", name, ErrorDuplicateTransactionName)
		}
		seenTxNames[name] = true

		id := transaction.TypeID(i + 1)
		if err := t.SetID(id); err != nil {
			return err
		}
	}
	return nil
}

func (w *World) registerInternalTransactions() {
	w.registeredTransactions = append(w.registeredTransactions,
		CreatePersonaTx,
		AuthorizePersonaAddressTx,
	)
}

func (w *World) ListReads() []IRead {
	return w.registeredReads
}

func (w *World) ListTransactions() ([]transaction.ITransaction, error) {
	if !w.isTransactionsRegistered {
		return nil, errors.New("cannot list transactions until transaction registration occurs")
	}
	return w.registeredTransactions, nil
}

// NewWorld creates a new world.
func NewWorld(s storage.WorldStorage, opts ...Option) (*World, error) {
	w := &World{
		store:     s,
		namespace: "world",
		tick:      0,
		systems:   make([]System, 0),
		txQueues:  transaction.TxMap{},
	}
	w.AddSystems(RegisterPersonaSystem, AuthorizePersonaAddressSystem)
	for _, opt := range opts {
		opt(w)
	}
	if w.receiptHistory == nil {
		w.receiptHistory = receipt.NewHistory(w.CurrentTick(), 10)
	}
	return w, nil
}

func (w *World) CurrentTick() uint64 {
	return w.tick
}

func (w *World) ReceiptHistorySize() uint64 {
	return w.receiptHistory.Size()
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

	// move component
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
func (w *World) copyTransactions() transaction.TxMap {
	w.txLock.Lock()
	defer w.txLock.Unlock()
	txsMap := make(transaction.TxMap, len(w.txQueues))
	for id, txs := range w.txQueues {
		txsMap[id] = make([]transaction.TxAny, len(txs))
		copy(txsMap[id], txs)
		w.txQueues[id] = w.txQueues[id][:0]
	}
	return txsMap
}

// AddTransaction adds a transaction to the transaction queue. This should not be used directly.
// Instead, use a TransactionType.AddToQueue to ensure type consistency. Returns the tick this transaction will be
// executed in.
func (w *World) AddTransaction(id transaction.TypeID, v any, sig *sign.SignedPayload) (tick uint64, txHash transaction.TxHash) {
	w.txLock.Lock()
	defer w.txLock.Unlock()
	txHash = transaction.TxHash(sig.HashHex())
	w.txQueues[id] = append(w.txQueues[id], transaction.TxAny{
		TxHash: txHash,
		Value:  v,
		Sig:    sig,
	})
	return w.CurrentTick(), txHash
}

// Tick performs one game tick. This consists of taking a snapshot of all pending transactions, then calling
// each System in turn with the snapshot of transactions.
func (w *World) Tick(ctx context.Context) error {
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
	w.receiptHistory.NextTick()

	if err := w.saveArchetypeData(); err != nil {
		return err
	}

	if err := w.store.TickStore.FinalizeTick(); err != nil {
		return err
	}
	w.tick++

	return nil
}

func (w *World) StartGameLoop(ctx context.Context, loopInterval time.Duration) {
	log.Info().Msg("Game loop started")
	go func() {
		for range time.Tick(loopInterval) {
			if err := w.Tick(ctx); err != nil {
				log.Panic().Err(err).Msg("Error running Tick in Game Loop.")
			}
		}
	}()
}

type TxBatch struct {
	TxID transaction.TypeID `json:"tx_id,omitempty"`
	Txs  []any              `json:"txs,omitempty"`
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
func (w *World) recoverGameState() (recoveredTxs transaction.TxMap, err error) {
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
	if !w.isTransactionsRegistered {
		if err := w.RegisterTransactions(); err != nil {
			return err
		}
	}
	if !w.isComponentsRegistered {
		if err := w.RegisterComponents(); err != nil {
			return err
		}
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
		if err := w.Tick(context.Background()); err != nil {
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
	// check if there are duplicate values inside component slice
	for i := 0; i < len(components); i++ {
		for j := i + 1; j < len(components); j++ {
			if components[i] == components[j] {
				return false
			}
		}
	}
	return true
}

// RecoverFromChain will attempt to recover the state of the world based on historical transaction data.
// The function puts the world in a recovery state, and then queries all transaction batches under the world's
// namespace. The function will continuously ask the EVM base shard for batches, and run ticks for each batch returned.
func (w *World) RecoverFromChain(ctx context.Context) error {
	if w.chain == nil {
		return fmt.Errorf("chain adapter was nil. " +
			"be sure to use the `WithAdapter` option when creating the world")
	}
	if w.tick > 0 {
		return fmt.Errorf("world recovery should not occur in a world with existing state. please verify all " +
			"state has been cleared before running recovery")
	}

	w.isRecovering = true
	defer func() {
		w.isRecovering = false
	}()
	namespace := w.Namespace()
	var nextKey []byte
	for {
		res, err := w.chain.QueryTransactions(ctx, &types.QueryTransactionsRequest{
			Namespace: namespace,
			Page: &types.PageRequest{
				Key: nextKey,
			},
		})
		if err != nil {
			return err
		}
		for _, tickedTxs := range res.Epochs {
			target := tickedTxs.Epoch
			// tick up to target
			if target < w.CurrentTick() {
				return fmt.Errorf("got tx for tick %d, but world is at tick %d", target, w.CurrentTick())
			}
			for current := w.CurrentTick(); current != target; {
				if err := w.Tick(ctx); err != nil {
					return err
				}
				current = w.CurrentTick()
			}
			// we've now reached target. we need to inject the transactions and tick.
			transactions := tickedTxs.Txs
			for _, tx := range transactions {
				sp, err := w.decodeTransaction(tx.SignedPayload)
				if err != nil {
					return err
				}
				itx := w.getITx(transaction.TypeID(tx.TxId))
				if itx == nil {
					return fmt.Errorf("error recovering tx with ID %d: tx id not found", tx.TxId)
				}
				v, err := itx.Decode(sp.Body)
				if err != nil {
					return err
				}
				w.AddTransaction(transaction.TypeID(tx.TxId), v, w.protoSignedPayloadToGo(sp))
			}
			// run the tick for this batch
			if err := w.Tick(ctx); err != nil {
				return err
			}
		}

		// if a page response was in the reply, that means there is more data to read.
		if res.Page != nil {
			// case where the next key is empty or nil, we don't want to continue the queries.
			if res.Page.Key == nil || len(res.Page.Key) == 0 {
				break
			}
			nextKey = res.Page.Key
		} else {
			// if the entire page reply is nil, then we are definitely done.
			break
		}
	}
	return nil
}

func (w *World) protoSignedPayloadToGo(sp *shardv1.SignedPayload) *sign.SignedPayload {
	return &sign.SignedPayload{
		PersonaTag: sp.PersonaTag,
		Namespace:  sp.Namespace,
		Nonce:      sp.Nonce,
		Signature:  sp.Signature,
		Body:       sp.Body,
	}
}

func (w *World) decodeTransaction(bz []byte) (*shardv1.SignedPayload, error) {
	payload := new(shardv1.SignedPayload)
	err := proto.Unmarshal(bz, payload)
	return payload, err
}

// getITx iterates over the registered transactions and returns the ITransaction associated with the TypeID.
func (w *World) getITx(id transaction.TypeID) transaction.ITransaction {
	var itx transaction.ITransaction
	for _, tx := range w.registeredTransactions {
		if id == tx.ID() {
			itx = tx
			break
		}
	}
	return itx
}

// Namespace returns the world's namespace.
func (w *World) Namespace() string {
	return string(w.namespace)
}

func (w *World) LogError(err error) {
	w.errs = append(w.errs, err)
}

func (w *World) GetNonce(signerAddress string) (uint64, error) {
	return w.store.NonceStore.GetNonce(signerAddress)
}

func (w *World) SetNonce(signerAddress string, nonce uint64) error {
	return w.store.NonceStore.SetNonce(signerAddress, nonce)
}

func (w *World) AddTransactionError(id transaction.TxHash, err error) {
	w.receiptHistory.AddError(id, err)
}

func (w *World) SetTransactionResult(id transaction.TxHash, a any) {
	w.receiptHistory.SetResult(id, a)
}

func (w *World) GetTransactionReceipt(id transaction.TxHash) (any, []error, bool) {
	rec, ok := w.receiptHistory.GetReceipt(id)
	if !ok {
		return nil, nil, false
	}
	return rec.Result, rec.Errs, true
}

func (w *World) GetTransactionReceiptsForTick(tick uint64) ([]receipt.Receipt, error) {
	return w.receiptHistory.GetReceiptsForTick(tick)
}
