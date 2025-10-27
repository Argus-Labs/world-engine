package cardinal

import (
	"reflect"
	"time"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/cardinal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/service"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
)

// ECS type aliases for easier user imports.
type (
	Exact[T any]         = ecs.Exact[T]
	Ref[T ecs.Component] = ecs.Ref[T]
	Contains[T any]      = ecs.Contains[T]
)

// RegisterSystem registers a system and its state with the world. By default, systems are registered to the
// Update hook. This can be overridden with the optional WithHook option.
//
// Example:
//
//	type RegenSystemState struct {
//		cardinal.BaseSystemState
//		Players ecs.Exact[struct {
//			PlayerTag ecs.Ref[PlayerTag]
//			Health    ecs.Ref[Health]
//		}]
//	}
//
//	world := cardinal.NewWorld()
//	cardinal.RegisterSystem(world, func(state *RegenSystemState) error {
//	    // System logic here
//	    return nil
//	})
func RegisterSystem[T any](w *World, system ecs.System[T], opts ...ecs.SystemOption) {
	// Check that the system state embeds BaseSystemState.
	var zero T
	state := reflect.TypeOf(zero)
	if _, ok := state.FieldByName("BaseSystemState"); !ok {
		panic(eris.Errorf("system %T must embed cardinal.BaseSystemState", system))
	}

	// Apply Cardinal specific system field modifiers.
	opts = append(opts,
		ecs.WithModifier(ecs.FieldBase, baseSystemStateInit(w)),
		ecs.WithModifier(ecs.FieldCommand, withCommandInit(w)),
		ecs.WithModifier(ecs.FieldEvent, withEventInit(w)),
		ecs.WithModifier(ecs.FieldSystemEventReceiver, withSystemEventReceiverInit(w)),
		ecs.WithModifier(ecs.FieldSystemEventEmitter, withSystemEventEmitterInit(w)),
	)
	ecs.RegisterSystem(w.getWorld(), system, opts...)
}

// The following aliases are exported from ecs so that users don't have to import the ecs package.

// PreUpdate runs before the main update.
const PreUpdate = ecs.PreUpdate

// Update runs during the main update phase.
const Update = ecs.Update

// PostUpdate runs after the main update.
const PostUpdate = ecs.PostUpdate

// Init runs once during world initialization.
const Init = ecs.Init

// WithHook returns an option to set the system hook.
func WithHook(hook ecs.SystemHook) ecs.SystemOption {
	return ecs.WithHook(hook)
}

// cardinalSystemStateField is an interface that cardinal's system field wrappers must implement.
type cardinalSystemStateField interface {
	init(*World) error
}

var _ cardinalSystemStateField = &BaseSystemState{}
var _ cardinalSystemStateField = &WithCommand[ecs.Command]{}
var _ cardinalSystemStateField = &WithEvent[ecs.Event]{}
var _ cardinalSystemStateField = &WithSystemEventReceiver[ecs.SystemEvent]{}
var _ cardinalSystemStateField = &WithSystemEventEmitter[ecs.SystemEvent]{}

// -----------------------------------------------------------------------------
// Base System State Field
// -----------------------------------------------------------------------------

// BaseSystemState provides common functionality for system state types. It should be embedded in your system state
// types to allow your systems to access the world state.
//
// Example:
//
//	// Define your system state by embedding BaseState.
//	type DebugSystemState struct {
//	    cardinal.BaseSystemState
//	    // Other fields...
//	}
//
//	// Your system function receives a pointer to your system state.
//	func DebugSystem(state *DebugSystemState) error {
//	    state.Logger().Debug().Int("tick", state.Tick()).Msg("...")
//	    return nil
//	}
type BaseSystemState struct {
	ecs.BaseSystemState
	cardinal *World
}

// init initializes the base system state field.
func (b *BaseSystemState) init(w *World) error {
	b.cardinal = w
	return nil
}

// Logger returns the logger for the world.
func (b *BaseSystemState) Logger() *zerolog.Logger {
	logger := b.cardinal.tel.GetLogger("system")
	return &logger
}

// Tick returns the current tick of the world.
func (b *BaseSystemState) Tick() uint64 {
	tick, err := b.cardinal.CurrentTick()
	assert.That(err == nil, "GetCurrentTick should never fail during system execution")
	return tick.Header.TickHeight
}

// Timestamp returns the current timestamp of the world.
func (b *BaseSystemState) Timestamp() time.Time {
	tick, err := b.cardinal.CurrentTick()
	assert.That(err == nil, "GetCurrentTick should never fail during system execution")
	return tick.Header.Timestamp
}

// TODO: Rand method, ScheduleTasks(?)

// baseSystemStateInit initializes the base system state field. It checks that the user is using
// cardinal.BaseSystemState instead of ecs.BaseSystemState.
func baseSystemStateInit(w *World) func(any) error {
	return func(field any) error {
		b, ok := field.(*BaseSystemState)
		if !ok {
			return eris.New("field must be cardinal.BaseSystemState")
		}
		return b.init(w)
	}
}

// -----------------------------------------------------------------------------
// Command Field
// -----------------------------------------------------------------------------

// WithCommand is a generic system state field that allows systems to receive commands of type T during each tick.
// Commands must embed a cardinal.BaseCommand that provides common methods for commands. The commands are automatically
// registered when the systems are registered.
//
// Example:
//
//	// Define a command type for spawning players.
//	type SpawnPlayerCommand struct {
//	    cardinal.BaseCommand
//	    Name string
//	}
//
//	func (SpawnPlayerCommand) Name() string { return "spawn-player" }
//
//	// Define your system state.
//	type SpawnSystemState struct {
//	    cardinal.BaseSystemState
//	    SpawnPlayerCommands cardinal.WithCommand[SpawnPlayerCommand]
//	    // Other fields...
//	}
//
//	// Your system function receives a pointer to your system state.
//	func SpawnSystem(state *SpawnSystemState) error {
//	    // Process all spawn commands for the current tick.
//	    for command := range state.SpawnPlayerCommands.Iter() {
//	        state.Players.Create(PlayerTag{Name: command.Name}, Health{Value: 100})
//	    }
//	    return nil
//	}
type WithCommand[T ecs.Command] struct {
	ecs.WithCommand[T]
}

// init initializes the command state field, registers the command with the command manager, and creates a shard service
// handler for it. Returns an error if the command doesn't embed cardinal.BaseCommand.
func (c *WithCommand[T]) init(w *World) error {
	var zero T
	name := zero.Name()

	// Use reflection to check if T embeds BaseCommand.
	commandType := reflect.TypeOf(zero)
	_, ok := commandType.FieldByName("BaseCommand")
	if !ok {
		return eris.Errorf("Command %s must embed cardinal.BaseCommand", name)
	}

	if err := micro.RegisterCommand[T](w.Shard); err != nil {
		return eris.Wrap(err, "failed to register command with shard")
	}

	return nil
}

// withCommandInit initializes the command state field. It checks that the user is using
// cardinal.WithCommand[T] instead of ecs.WithCommand[T].
func withCommandInit(w *World) func(any) error {
	return func(field any) error {
		c, ok := field.(cardinalSystemStateField)
		if !ok {
			return eris.New("field must be cardinal.WithCommand[T]")
		}
		return c.init(w)
	}
}

// BaseCommand is a base command type that all commands should embed.
type BaseCommand struct{}

// -----------------------------------------------------------------------------
// Event Field
// -----------------------------------------------------------------------------

// WithEvent is a generic system state field that allows systems to emit events of type T. Events must embed a
// cardinal.BaseEvent that provides common methods for events. At the end of each tick, events are published to their
// respective subjects.
//
// Example:
//
//	// Define an event type for player deaths.
//	type LevelUpEvent struct {
//	    cardinal.BaseEvent
//	    Nickname string
//	}
//
//	func (LevelUpEvent) Name() string { return "level-up" }
//
//	type LevelUpSystemState struct {
//	    cardinal.BaseSystemState
//	    LevelUpEvents cardinal.WithEvent[LevelUpEvent]
//	    // Other fields...
//	}
//
//	// Emit a level up event from one system.
//	func LevelUpSystem(state *LevelUpSystemState) error {
//	    state.LevelUpEvents.Emit(LevelUpEvent{Nickname: "Player1"})
//	    return nil
//	}
type WithEvent[T ecs.Event] struct {
	ecs.WithEvent[T]
}

// init initializes the event state field. It checks that the event type embeds cardinal.BaseEvent.
func (e *WithEvent[T]) init(_ *World) error {
	var zero T
	name := zero.Name()

	// Use reflection to check if T embeds BaseEvent.
	eventType := reflect.TypeOf(zero)
	_, ok := eventType.FieldByName("BaseEvent")
	if !ok {
		return eris.Errorf("Event %s must embed cardinal.BaseEvent", name)
	}
	return nil
}

// withEventInit initializes the event state field. It checks that the user is using cardinal.WithEvent[T]
// instead of ecs.WithEvent[T].
func withEventInit(w *World) func(any) error {
	return func(field any) error {
		e, ok := field.(cardinalSystemStateField)
		if !ok {
			return eris.New("field must be cardinal.WithEvent[T]")
		}
		return e.init(w)
	}
}

// BaseEvent is a base event type that all events should embed.
type BaseEvent struct{}

// -----------------------------------------------------------------------------
// System Event Field
//
// These aren't used atm to add extra functionality on top of ecs's sytem event fields, but if we
// do need to do it one day, we can just update the init methods here.
// -----------------------------------------------------------------------------

// WithSystemEventReceiver is a generic system state field that allows systems to receive system events of type T.
// System events don't have to be registered, and can be used to communicate between systems.
//
// Example:
//
//	// Define a system event for player deaths.
//	type PlayerDeathEvent struct { Nickname string }
//
//	func (PlayerDeathEvent) Name() string { return "player-death" }
//
//	type GraveyardSystemState struct {
//	    cardinal.BaseSystemState
//	    PlayerDeathEvents cardinal.WithSystemEventReceiver[PlayerDeathEvent]
//	    // Other fields...
//	}
//
//	// Receive a player death event from another system.
//	func GraveyardSystem(state *GraveyardSystemState) error {
//	    for event := range state.PlayerDeathEvents.Iter() {
//	        state.Logger().Info().Msgf("Player %s died", event.Nickname)
//	    }
//	    return nil
//	}
type WithSystemEventReceiver[T ecs.SystemEvent] struct {
	ecs.WithSystemEventReceiver[T]
}

// init initializes the system event receiver state field. Does nothing atm as we don't have any custom behavior
// for system events.
func (e *WithSystemEventReceiver[T]) init(_ *World) error {
	return nil
}

// withSystemEventReceiverInit initializes the system event receiver state field. It checks that the user is using
// cardinal.WithSystemEventReceiver[T] instead of ecs.WithSystemEventReceiver[T].
func withSystemEventReceiverInit(w *World) func(any) error {
	return func(field any) error {
		e, ok := field.(cardinalSystemStateField)
		if !ok {
			return eris.New("field must be cardinal.WithSystemEventReceiver[T]")
		}
		return e.init(w)
	}
}

// WithSystemEventEmitter is a generic system state field that allows systems to emit system events of type T.
// System events don't have to be registered, and can be used to communicate between systems.
//
// Example:
//
//	// Define a system event for player deaths.
//	type PlayerDeathEvent struct { Nickname string }
//
//	func (PlayerDeathEvent) Name() string { return "player-death" }
//
//	type CombatSystemState struct {
//	    PlayerDeathEvents ecs.WithSystemEventEmitter[PlayerDeathEvent]
//	    // Other fields...
//	}
//
//	// Emit a player death event from one system.
//	func CombatSystem(state *CombatSystemState) error {
//	    state.PlayerDeathEvents.Emit(PlayerDeathEvent{Nickname: "Player1"})
//	    return nil
//	}
type WithSystemEventEmitter[T ecs.SystemEvent] struct {
	ecs.WithSystemEventEmitter[T]
}

// init initializes the system event emitter state field. Does nothing atm as we don't have any custom behavior
// for system events.
func (e *WithSystemEventEmitter[T]) init(_ *World) error {
	return nil
}

// withSystemEventEmitterInit initializes the system event emitter state field. It checks that the user is using
// cardinal.WithSystemEventEmitter[T] instead of ecs.WithSystemEventEmitter[T].
func withSystemEventEmitterInit(w *World) func(any) error {
	return func(field any) error {
		e, ok := field.(cardinalSystemStateField)
		if !ok {
			return eris.New("field must be cardinal.WithSystemEventEmitter[T]")
		}
		return e.init(w)
	}
}

// -----------------------------------------------------------------------------
// Inter-Shard Commands
// -----------------------------------------------------------------------------

// OtherWorld is a type that represents the address of an external service.
type OtherWorld struct {
	Region       string
	Organization string
	Project      string
	ShardID      string
}

// Send sends a command to an external service.
//
// Example:
//
// import external "another-game-shard/system"
//
// // Define a shard address of one of your game shards.
// const MatchmakingService ecs.ServiceAddress = "world.argus.rampage.matchmaking"
//
//	func GameSystem(state *GameSystem) error {
//	  MatchmakingService.Send(state, &external.EndGameCommand{
//	    Winner: "Team 1",
//	  })
//	}
func (o OtherWorld) Send(state *BaseSystemState, command ecs.Command) {
	serviceAddress := micro.GetAddress(o.Region, micro.RealmWorld, o.Organization, o.Project, o.ShardID)
	state.EmitRawEvent(service.EventKindInterShardCommand, service.InterShardCommand{
		Target:  serviceAddress,
		Command: command,
	})
}
