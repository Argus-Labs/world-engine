package cardinal

import (
	"math/rand"
	"reflect"
	"time"

	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/persona/component"
	"pkg.world.dev/world-engine/cardinal/receipt"
	"pkg.world.dev/world-engine/cardinal/txpool"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/worldstage"
	"pkg.world.dev/world-engine/sign"
)

// interface guard.
var _ WorldContext = (*worldContext)(nil)

//go:generate mockgen -source=context.go -package mocks -destination=mocks/context.go
type WorldContext interface {
	// Timestamp returns the UNIX timestamp of the tick in milliseconds.
	// Millisecond is used to provide precision when working with subsecond tick intervals.
	Timestamp() uint64

	// CurrentTick returns the current tick.
	CurrentTick() uint64

	// Logger returns the logger that can be used to log messages from within system or query.
	Logger() *zerolog.Logger

	// EmitEvent emits an event that will be broadcast to all websocket subscribers.
	EmitEvent(map[string]any) error

	// EmitStringEvent emits a string event that will be broadcast to all websocket subscribers.
	// This method is provided for backwards compatability. EmitEvent should be used for most cases.
	EmitStringEvent(string) error

	// Namespace returns the namespace of the world.
	Namespace() string

	// Rand returns a random number generator that is seeded specifically for a current tick.
	Rand() *rand.Rand

	// ScheduleTickTask schedules a task to be executed after the specified tickDelay.
	// The given Task must have been registered using RegisterTask.
	ScheduleTickTask(uint64, Task) error

	// ScheduleTimeTask schedules a task to be executed after the specified duration (in wall clock time).
	// The given Task must have been registered using RegisterTask.
	ScheduleTimeTask(time.Duration, Task) error

	// GetAllEntities returns all entities and their components as a map.
	// The map is keyed by entity ID, and the value is a map of component name to component data.
	GetAllEntities() (map[types.EntityID]map[string]any, error)

	// Private methods for internal use.
	setLogger(logger zerolog.Logger)
	addMessageError(id types.TxHash, err error)
	setMessageResult(id types.TxHash, a any)
	getComponentByName(name string) (types.ComponentMetadata, error)
	getMessageByType(mType reflect.Type) (types.Message, bool)
	getTransactionReceipt(id types.TxHash) (any, []error, bool)
	getSignerForPersonaTag(personaTag string, tick uint64) (addr string, err error)
	getTransactionReceiptsForTick(tick uint64) ([]receipt.Receipt, error)
	receiptHistorySize() uint64
	addTransaction(id types.MessageID, v any, sig *sign.Transaction) (uint64, types.TxHash)
	isWorldReady() bool
	storeReader() gamestate.Reader
	storeManager() gamestate.Manager
	getTxPool() *txpool.TxPool
	isReadOnly() bool
}

type worldContext struct {
	world    *World
	txPool   *txpool.TxPool
	logger   *zerolog.Logger
	readOnly bool
	rand     *rand.Rand
}

func newWorldContextForTick(world *World, txPool *txpool.TxPool) WorldContext {
	return &worldContext{
		world:    world,
		txPool:   txPool,
		logger:   &log.Logger,
		readOnly: false,
		//nolint:gosec // we require manual in the rng which crypto/rand doesn't have, but math/rand does.
		rand: rand.New(rand.NewSource(int64(world.timestamp.Load()))),
	}
}

func NewWorldContext(world *World) WorldContext {
	return &worldContext{
		world:    world,
		txPool:   nil,
		logger:   &log.Logger,
		readOnly: false,
		rand:     nil,
	}
}

func NewReadOnlyWorldContext(world *World) WorldContext {
	return &worldContext{
		world:    world,
		txPool:   nil,
		logger:   &log.Logger,
		readOnly: true,
		rand:     nil,
	}
}

// -----------------------------------------------------------------------------
// Public methods
// -----------------------------------------------------------------------------

func (ctx *worldContext) ScheduleTickTask(tickDelay uint64, task Task) error {
	triggerAtTick := ctx.CurrentTick() + tickDelay
	return createTickTask(ctx, triggerAtTick, task)
}

func (ctx *worldContext) ScheduleTimeTask(duration time.Duration, task Task) error {
	if duration.Milliseconds() < 0 {
		return eris.New("duration value must be positive")
	}

	triggerAtTimestamp := ctx.Timestamp() + uint64(duration.Milliseconds()) //nolint:gosec
	return createTimestampTask(ctx, triggerAtTimestamp, task)
}

func (ctx *worldContext) EmitEvent(event map[string]any) error {
	return ctx.world.tickResults.AddEvent(event)
}

func (ctx *worldContext) EmitStringEvent(e string) error {
	return ctx.world.tickResults.AddStringEvent(e)
}

func (ctx *worldContext) Timestamp() uint64 {
	return ctx.world.timestamp.Load()
}

func (ctx *worldContext) CurrentTick() uint64 {
	return ctx.world.CurrentTick()
}

func (ctx *worldContext) Logger() *zerolog.Logger {
	return ctx.logger
}

func (ctx *worldContext) Rand() *rand.Rand {
	if ctx.rand == nil {
		// a panic is thrown here instead of returning an error to maintain method chaining.
		// ex: wCtx.rand().Int63()
		panic(eris.New("rand is only useable on a context generated by newWorldContextForTick"))
	}
	return ctx.rand
}

func (ctx *worldContext) Namespace() string {
	return ctx.world.Namespace()
}

// GetAllEntities returns all entities and their components as a map.
// The map is keyed by entity ID, and the value is a map of component name to component data.
func (ctx *worldContext) GetAllEntities() (map[types.EntityID]map[string]any, error) {
	entities := make(map[types.EntityID]map[string]any)

	// Get all entities excluding internal Persona components
	err := NewSearch().Entity(
		filter.Not(
			filter.Or(
				filter.Contains(filter.Component[component.SignerComponent]()),
				filter.Contains(filter.Component[taskMetadata]()),
			),
		),
	).Each(ctx, func(id types.EntityID) bool {
		entities[id] = make(map[string]any)

		components, err := ctx.world.StoreReader().GetComponentTypesForEntity(id)
		if err != nil {
			return false
		}

		for _, c := range components {
			compJSON, err := ctx.world.StoreReader().GetComponentForEntityInRawJSON(c, id)
			if err != nil {
				return false
			}
			entities[id][c.Name()] = compJSON
		}
		return true
	})
	if err != nil {
		return nil, err
	}

	return entities, nil
}

// -----------------------------------------------------------------------------
// Private methods
// -----------------------------------------------------------------------------

func (ctx *worldContext) getMessageByType(mType reflect.Type) (types.Message, bool) {
	return ctx.world.GetMessageByType(mType)
}

func (ctx *worldContext) setLogger(logger zerolog.Logger) {
	ctx.logger = &logger
}

func (ctx *worldContext) getComponentByName(name string) (types.ComponentMetadata, error) {
	return ctx.world.GetComponentByName(name)
}

func (ctx *worldContext) addMessageError(id types.TxHash, err error) {
	// TODO(scott): i dont trust exposing this to the users. this should be fully abstracted away.
	ctx.world.receiptHistory.AddError(id, err)
}

func (ctx *worldContext) setMessageResult(id types.TxHash, a any) {
	// TODO(scott): i dont trust exposing this to the users. this should be fully abstracted away.
	ctx.world.receiptHistory.SetResult(id, a)
}

func (ctx *worldContext) getTransactionReceipt(id types.TxHash) (any, []error, bool) {
	rec, ok := ctx.world.receiptHistory.GetReceipt(id)
	if !ok {
		return nil, nil, false
	}
	return rec.Result, rec.Errs, true
}

func (ctx *worldContext) getSignerForPersonaTag(personaTag string, tick uint64) (addr string, err error) {
	return ctx.world.GetSignerForPersonaTag(personaTag, tick)
}

func (ctx *worldContext) getTransactionReceiptsForTick(tick uint64) ([]receipt.Receipt, error) {
	return ctx.world.GetTransactionReceiptsForTick(tick)
}

func (ctx *worldContext) receiptHistorySize() uint64 {
	return ctx.world.receiptHistory.Size()
}

func (ctx *worldContext) addTransaction(id types.MessageID, v any, sig *sign.Transaction) (uint64, types.TxHash) {
	return ctx.world.AddTransaction(id, v, sig)
}

func (ctx *worldContext) getTxPool() *txpool.TxPool {
	return ctx.txPool
}

func (ctx *worldContext) isReadOnly() bool {
	return ctx.readOnly
}

func (ctx *worldContext) storeManager() gamestate.Manager {
	return ctx.world.entityStore
}

func (ctx *worldContext) storeReader() gamestate.Reader {
	sm := ctx.storeManager()
	if ctx.isReadOnly() {
		return sm.ToReadOnly()
	}
	return sm
}

func (ctx *worldContext) isWorldReady() bool {
	stage := ctx.world.worldStage.Current()
	return stage == worldstage.Ready ||
		stage == worldstage.Running ||
		stage == worldstage.Recovering
}
