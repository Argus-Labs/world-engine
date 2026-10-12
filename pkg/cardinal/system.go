package cardinal

import (
	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
)

type EntityID = ecs.EntityID

// System is a unit of game logic that runs once per tick phase. Run receives the world it was
// registered with; use it for searches, commands, events, entities, and the current tick. Any
// other state a system needs (a runtime, a config) lives on the implementing struct.
type System interface {
	Run(w *World)
}

// ErrWorldStarted is the panic value of every Register* method once init has run the first system.
// Registration mutates tables that systems read (component IDs, command queues, event and system
// event registries), so it must finish before StartGame and cannot happen inside a system. This is
// a plain panic, not assert.That, so it also fires in release builds.
var ErrWorldStarted = eris.New(
	"cannot register after the world has started; register before StartGame and outside systems")

// -------------------------------------------------------------------------------------------------
// Options
// -------------------------------------------------------------------------------------------------

// systemConfig holds all configurable options for system registration.
type systemConfig struct {
	// The hook that determines when the system should be executed.
	hook ecs.SystemHook
}

// SystemOption is a function that configures a SystemConfig.
type SystemOption func(*systemConfig)

// SystemHook defines when a system should be executed in the update cycle.
type SystemHook = ecs.SystemHook

const (
	// PreUpdate runs before the main update.
	PreUpdate = ecs.PreUpdate
	// Update runs during the main update phase.
	Update = ecs.Update
	// PostUpdate runs after the main update.
	PostUpdate = ecs.PostUpdate
	// Init runs once during world initialization.
	Init = ecs.Init
)

// WithHook returns an option to set the system hook.
func WithHook(hook SystemHook) SystemOption {
	return func(cfg *systemConfig) { cfg.hook = hook }
}

// -------------------------------------------------------------------------------------------------
// Commands
// -------------------------------------------------------------------------------------------------

type Command = command.Payload

// CommandContext is a command and its sender. The sender is a player, whose ID Cardinal takes from
// the client's auth token, or another shard that called SendToShard. Ask which with Player or Shard:
//
//	if player, ok := cmd.Player(); ok {
//		w.SendTo(player, Result{OK: true})
//	}
type CommandContext[T Command] struct {
	Payload T
	sender  command.Sender
}

// Sender returns the player ID or the sending shard's address. Use it when any sender will do, such
// as in logs.
func (c CommandContext[T]) Sender() string { return c.sender.ID() }

// Player returns the ID of the player who sent the command, or false if a shard sent it.
func (c CommandContext[T]) Player() (string, bool) { return c.sender.Player() }

// Shard returns the sending shard's address ("region.realm.org.project.shard"), or false if a
// player sent it.
func (c CommandContext[T]) Shard() (string, bool) { return c.sender.Shard() }

// -------------------------------------------------------------------------------------------------
// Inter-Shard Commands
// -------------------------------------------------------------------------------------------------

// OtherWorld is a type that represents the address of an external service.
type OtherWorld struct {
	Region       string
	Organization string
	Project      string
	ShardID      string
}

// -------------------------------------------------------------------------------------------------
// Events
// -------------------------------------------------------------------------------------------------

type Event = event.Payload

// -------------------------------------------------------------------------------------------------
// Search
// -------------------------------------------------------------------------------------------------

// Search is a resolved, world-bound query returned by World.Contains and World.Exact.
// It holds component IDs, not entity data, so it remains valid as entities change.
type Search struct {
	world      *ecs.World
	components bitmap.Bitmap
	match      ecs.SearchMatch
}

// Iter yields each matching entity as a world-bound handle.
func (q Search) Iter() SearchResult {
	return func(yield func(Entity) bool) {
		err := q.world.IterEntities(q.components, q.match, func(eid EntityID) bool {
			return yield(Entity{world: q.world, id: eid})
		})
		assert.That(err == nil, "invalid arguments sent to IterEntities")
	}
}

// GetByID returns a handle if the entity matches the query's archetype.
func (q Search) GetByID(eid EntityID) (Entity, error) {
	if err := q.world.MatchArchetype(eid, q.components, q.match); err != nil {
		return Entity{}, eris.Wrap(err, "failed to get entity")
	}
	return Entity{world: q.world, id: eid}, nil
}

// Create returns a new entity with the query's components, initialized to zero.
func (q Search) Create() Entity {
	return Entity{world: q.world, id: q.world.CreateWithArchetype(q.components)}
}

// -------------------------------------------------------------------------------------------------
// Component Search Result Modifiers
// -------------------------------------------------------------------------------------------------

var (
	ErrSingleNoResult       = eris.New("expected exactly 1 result, got 0")
	ErrSingleMultipleResult = eris.New("expected exactly 1 result, got more than 1")
)

// SearchResult is a chainable iterator over entities.
type SearchResult func(yield func(Entity) bool)

// Filter returns a new iterator that only yields values that satisfy predicate. A nil predicate
// returns the original iterator unchanged.
func (s SearchResult) Filter(predicate func(Entity) bool) SearchResult {
	if predicate == nil {
		return s
	}

	return func(yield func(Entity) bool) {
		s(func(c Entity) bool {
			return !predicate(c) || yield(c)
		})
	}
}

// Limit returns a new iterator that yields at most limit values. A limit <= 0 yields no values.
func (s SearchResult) Limit(limit uint32) SearchResult {
	return func(yield func(Entity) bool) {
		if limit == 0 {
			return
		}
		yielded := uint32(0)
		s(func(c Entity) bool {
			yielded++
			return yield(c) && yielded < limit
		})
	}
}

// Single returns the single value in the iterator. It returns an error if the iterator yields
// zero or more than one result.
func (s SearchResult) Single() (Entity, error) {
	// A function-valued iterator can retain its callback. Keep the captured result
	// together so escape analysis needs one record rather than separate captured allocations.
	result := struct {
		entity Entity
		err    error
	}{err: ErrSingleNoResult}
	s(func(c Entity) bool {
		if result.err == nil {
			result.err = ErrSingleMultipleResult
			return false
		}
		result.entity, result.err = c, nil
		return true
	})
	return result.entity, result.err
}

// -------------------------------------------------------------------------------------------------
// Re-exported immutable types
// -------------------------------------------------------------------------------------------------

// Slice is world-engine's sequence with a private backing array, re-exported so a component can
// declare one without a second import. It is an alias, so cardinal.Slice[T] and immutable.Slice[T]
// are the same type and either spelling works.
//
// Its derivations edit in place; see immutable.Slice for what that means at a call site.
type Slice[T any] = immutable.Slice[T]

// SliceOf returns a Slice holding a copy of items. See immutable.SliceOf.
func SliceOf[T any](items ...T) Slice[T] { return immutable.SliceOf(items...) }
