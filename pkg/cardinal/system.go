package cardinal

import (
	"fmt"
	"iter"
	"reflect"
	"time"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
)

func RegisterSystem[T any](world *World, system func(*T), opts ...SystemOption) {
	cfg := newSystemConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	// Check that the system stateType embeds BaseSystemState.
	var zero T
	stateType := reflect.TypeOf(zero)
	if _, ok := stateType.FieldByName("BaseSystemState"); !ok {
		panic(eris.Errorf("system %T must embed cardinal.BaseSystemState", system))
	}

	// Initialize the fields in the system state.
	state := new(T)

	err := initSystemFields(state, world)
	if err != nil {
		panic(eris.Wrapf(err, "error initializing system fields"))
	}

	name := fmt.Sprintf("%T", system)
	systemFn := func() { system(state) }

	err = ecs.RegisterSystem(world.world, state, name, systemFn, cfg.hook)
	if err != nil {
		panic(eris.Wrapf(err, "error registering system"))
	}
}

func initSystemFields[T any](state *T, world *World) error {
	meta := systemInitMetadata{
		world:    world,
		commands: make(map[string]struct{}),
		events:   make(map[string]struct{}),
	}

	// For each field in the system state, initialize the field and collect its dependencies.
	value := reflect.ValueOf(state).Elem()
	for i := range value.NumField() {
		field := value.Field(i)
		fieldType := value.Type().Field(i)

		// If the field is not exported, return an error.
		if !field.CanAddr() {
			return eris.Errorf("field %s must be exported", fieldType.Name)
		}

		fieldInstance := field.Addr().Interface()

		cardinalField, ok := fieldInstance.(systemField)
		if ok {
			if err := cardinalField.init(&meta); err != nil {
				return eris.Wrapf(err, "failed to initialize field %s", fieldType.Name)
			}
			continue
		}

		// ECS fields will be initialized separately, so we just have to check the rest of the fields
		// are valid system field types.
		if _, isECSField := fieldInstance.(ecs.SystemField); !isECSField {
			return eris.Errorf("field %s is not a valid cardinal system field", fieldType.Name)
		}
	}
	return nil
}

type systemInitMetadata struct {
	world    *World
	commands map[string]struct{}
	events   map[string]struct{}
}

type systemField interface {
	init(meta *systemInitMetadata) error
}

var _ systemField = (*BaseSystemState)(nil)
var _ systemField = (*WithCommand[Command])(nil)
var _ systemField = (*WithEvent[Event])(nil)

// -------------------------------------------------------------------------------------------------
// Options
// -------------------------------------------------------------------------------------------------

// systemConfig holds all configurable options for system registration.
type systemConfig struct {
	// The hook that determines when the system should be executed.
	hook ecs.SystemHook
}

// newSystemConfig creates a new system config with default values.
func newSystemConfig() systemConfig {
	return systemConfig{
		hook: Update,
	}
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
// Base
// -------------------------------------------------------------------------------------------------

type BaseSystemState struct {
	world *World
}

func (b *BaseSystemState) init(meta *systemInitMetadata) error {
	b.world = meta.world
	return nil
}

// TODO: pass init args (similar to boot info) to get system name in logger.
// Logger returns the logger for the world.
func (b *BaseSystemState) Logger() *zerolog.Logger {
	logger := b.world.tel.GetLogger("system")
	return &logger
}

// Tick returns the current tick of the world.
func (b *BaseSystemState) Tick() uint64 {
	return b.world.currentTick.height
}

// Timestamp returns the current timestamp of the world.
func (b *BaseSystemState) Timestamp() time.Time {
	return b.world.currentTick.timestamp
}

// -------------------------------------------------------------------------------------------------
// Commands
// -------------------------------------------------------------------------------------------------

type Command = command.Payload

type WithCommand[T Command] struct {
	manager *command.Manager
	id      command.ID
}

func (c *WithCommand[T]) init(meta *systemInitMetadata) error {
	var zero T
	name := zero.Name()

	if _, ok := meta.commands[name]; ok {
		return eris.Errorf("systems cannot process multiple commands of the same type: %s", name)
	}

	id, err := meta.world.commands.Register(name, command.NewQueue[T]())
	if err != nil {
		return eris.Wrapf(err, "failed to register command %s", name)
	}

	// Register the command handler with NATS. NOTE: this just adds to the service's command name set,
	// it doesn't create the NATS subscription/request handler immediately. This method is free of
	// side effects so we can test without NATS.
	meta.world.service.registerCommandHandler(name)

	if err := meta.world.debug.register("command", zero); err != nil {
		return eris.Wrapf(err, "failed to register command to debug module %s", name)
	}

	meta.commands[name] = struct{}{} // Add to system commands set for duplicate field check

	c.manager = &meta.world.commands
	c.id = id
	return nil
}

func (c *WithCommand[T]) Iter() iter.Seq[CommandContext[T]] {
	var zero T
	commands, err := c.manager.Get(c.id)
	assert.That(err == nil, "command not automatically registered %s", zero.Name())

	return func(yield func(CommandContext[T]) bool) {
		for _, cmd := range commands {
			if !yield(newCommandContext[T](cmd)) {
				return
			}
		}
	}
}

type CommandContext[T Command] struct {
	Payload T
	Persona string
}

func newCommandContext[T Command](cmd command.Command) CommandContext[T] {
	payload, ok := cmd.Payload.(T)
	assert.That(ok, "mismatched command type passed to command context")

	return CommandContext[T]{
		Payload: payload,
		Persona: cmd.Persona,
	}
}

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

// SendCommand sends a command to an external service.
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
func (o OtherWorld) SendCommand(state *BaseSystemState, cmd command.Payload) {
	serviceAddress := micro.GetAddress(o.Region, micro.RealmWorld, o.Organization, o.Project, o.ShardID)
	state.world.events.Enqueue(event.Event{
		Kind: event.KindInterShardCommand,
		Payload: command.Command{
			Name:    cmd.Name(),
			Persona: micro.String(state.world.address),
			Address: serviceAddress,
			Payload: cmd,
		},
	})
}

// -------------------------------------------------------------------------------------------------
// Events
// -------------------------------------------------------------------------------------------------

type Event = event.Payload

type WithEvent[T Event] struct {
	manager *event.Manager
}

func (e *WithEvent[T]) init(meta *systemInitMetadata) error {
	var zero T
	name := zero.Name()

	if _, ok := meta.events[name]; ok {
		return eris.Errorf("systems cannot process multiple events of the same type: %s", name)
	}

	if err := meta.world.debug.register("event", zero); err != nil {
		return eris.Wrapf(err, "failed to register command to debug module %s", name)
	}

	meta.events[name] = struct{}{} // Add to system events set for duplicate field check

	e.manager = &meta.world.events
	return nil
}

func (e *WithEvent[T]) Emit(evt T) {
	e.manager.Enqueue(event.Event{
		Kind:    event.KindDefault,
		Payload: evt,
	})
}

type (
	Exact[T any]                               = ecs.Exact[T]
	Contains[T any]                            = ecs.Contains[T]
	Ref[T ecs.Component]                       = ecs.Ref[T]
	WithSystemEventReceiver[T ecs.SystemEvent] = ecs.WithSystemEventReceiver[T]
	WithSystemEventEmitter[T ecs.SystemEvent]  = ecs.WithSystemEventEmitter[T]
	EntityID                                   = ecs.EntityID
)
