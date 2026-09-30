package cardinal

import (
	"context"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rs/zerolog"
)

// TestWorld runs a game's systems inside a Go test, one step at a time. It embeds *World, so tests
// seed and read state with the normal API (w.Create, w.Entity, w.Contains, w.EmitSystemEvent).
//
//	func TestCombat(t *testing.T) {
//	    w := cardinal.NewTestWorld(t, game.Register)
//	    target := w.Create[game.Player]()
//	    w.Command("alice", game.Attack{Target: target.ID(), Damage: 30})
//	    w.RunSystem(&system.Combat{})
//	    assert.Equal(t, game.Health{HP: 70}, target.Get[game.Health]())
//	}
//
// A step is RunSystem or Tick. Each step drains the queued commands, dispatches and encodes events
// and shard commands, encodes the world state as a snapshot would, and increments the tick height,
// exactly as a production tick does. Events and ShardCommands return what the last step dispatched,
// which is everything sent since the step before it, including sends from Init systems and from the
// test itself. Emitted returns the system events the last step's systems emitted.
//
// Two methods shadow World's:
//   - Tick runs one production tick at the test clock. w.World.Tick(timestamp) runs one at another
//     time but is not a step: Events and ShardCommands add its outputs to the last step's, and
//     Emitted does not record its system events.
//   - StartGame fails the test. A TestWorld never connects to NATS.
//
// Everything runs on the test goroutine. A step at tick height h runs at [time.Unix](h, 0), and so
// do Init systems at height 0. The height increments at the end of each step, so between steps
// Timestamp is the last step's time, one second behind TickHeight.
type TestWorld struct {
	*World
	tb testing.TB

	// Outputs of the last step.
	events        []event.Event
	shardCommands []command.Command
	emitted       []ecs.SystemEvent
}

// Sent is an event that a step delivered with Broadcast or SendTo.
type Sent[T Event] struct {
	Recipient string // "" for Broadcast
	Payload   T
}

// ShardCommand is a command that a step sent to another shard with SendToShard.
type ShardCommand[T Command] struct {
	To      OtherWorld
	Payload T
}

// NewTestWorld builds a world for testing systems. setup registers components, commands, events,
// system events and systems, as a game's main does before StartGame. NewTestWorld then runs the init
// systems, as StartGame does, and closes registration.
//
// The world reads no environment variables and starts no NATS connection, telemetry exporter or
// snapshot storage. Logs go to the test log.
func NewTestWorld(t testing.TB, setup func(w *World)) *TestWorld {
	t.Helper()

	tel := telemetry.Telemetry{Logger: zerolog.New(zerolog.NewTestWriter(t))}
	storage := snapshot.NewNopStorage()
	w := &World{
		world:    ecs.NewWorld(),
		commands: command.NewManager(),
		events:   event.NewManager(1024),
		// SendToShard stamps the sender's address on every command, so the world needs one.
		address:         micro.GetAddress("test", micro.RealmWorld, "test", "test", "test"),
		snapshotStorage: storage,
		snapshotWriter:  snapshot.NewSyncWriter(storage, tel.GetLogger("snapshot")),
		tickCtx:         context.Background(),
		// Encode the state every step, so a component that cannot be serialized fails the step that
		// stored it rather than a production snapshot.
		options: WorldOptions{SnapshotRate: 1},
		tel:     tel,
	}
	// RegisterCommand records the command with the service. The service is never started.
	w.service = newService(w, AuthModeDev, "")

	tw := &TestWorld{World: w, tb: t}
	// Both handlers keep what a receiver would get: the payload encoded as the service does when it
	// publishes, then decoded. A payload that cannot be serialized fails the step that sent it rather
	// than a production tick, and a system that edits a value after sending it does not change the
	// recorded output.
	w.events.RegisterHandler(event.KindDefault, func(_ context.Context, evt event.Event) error {
		payload, ok := evt.Payload.(event.Payload)
		assert.That(ok, "event payload is %T, want event.Payload", evt.Payload)
		evt.Payload = roundTrip(payload)
		tw.events = append(tw.events, evt)
		return nil
	})
	w.events.RegisterHandler(event.KindInterShardCommand, func(_ context.Context, evt event.Event) error {
		cmd, ok := evt.Payload.(command.Command)
		assert.That(ok, "inter-shard command payload is %T, want command.Command", evt.Payload)
		cmd.Payload = roundTrip(cmd.Payload)
		tw.shardCommands = append(tw.shardCommands, cmd)
		return nil
	})

	setup(w)

	w.currentTick.timestamp = tw.now()
	w.init()
	return tw
}

// roundTrip encodes p and decodes the bytes into a fresh value of the same type.
func roundTrip[T schema.Serializable](p T) T {
	decoded, err := p.UnmarshalWire(schema.Marshal(p))
	assert.That(err == nil, "decode %s: %v", p.Name(), err)
	typed, ok := decoded.(T)
	assert.That(ok, "decode %s returned %T, want %T", p.Name(), decoded, p)
	return typed
}

// Command sends cmd from persona the way a client does. The command is encoded, decoded by its
// queue and read by Commands[T] during the next step only. It fails the test if the command type was
// not registered in setup.
func (w *TestWorld) Command(persona string, cmd Command) {
	w.tb.Helper()
	err := w.commands.Enqueue(context.Background(), &iscv1.Command{
		Name:    cmd.Name(),
		Address: w.address,
		Persona: &iscv1.Persona{Id: persona},
		Payload: schema.Marshal(cmd),
	})
	if err != nil {
		w.tb.Fatalf("cardinal: Command %s: %v", cmd.Name(), err)
	}
}

// RunSystem runs exactly one system as a step. The system need not be registered. System events
// emitted before the step are visible to it and are cleared when it returns, as at the end of a tick.
func (w *TestWorld) RunSystem(s System) {
	w.runStep(func() {
		defer w.world.ClearSystemEvents()
		s.Run(w.World)
	})
}

// Tick runs one production tick as a step: every PreUpdate, Update and PostUpdate system in order.
func (w *TestWorld) Tick() {
	w.runStep(w.world.Tick)
}

// StartGame fails the test. A TestWorld has no NATS connection or run loop; drive it with Tick or
// RunSystem.
func (w *TestWorld) StartGame() {
	w.tb.Helper()
	w.tb.Fatal("cardinal: StartGame called on a TestWorld; drive it with Tick or RunSystem")
}

// Events returns the events of type T that the last step dispatched, in the order they were sent.
func (w *TestWorld) Events[T Event]() []Sent[T] {
	var sent []Sent[T]
	for _, evt := range w.events {
		if payload, ok := evt.Payload.(T); ok {
			sent = append(sent, Sent[T]{Recipient: evt.Recipient, Payload: payload})
		}
	}
	return sent
}

// Emitted returns the system events of type T that systems emitted during the last step, in order.
// Events the test emitted before the step are inputs and are not included.
func (w *TestWorld) Emitted[T ecs.SystemEvent]() []T {
	var emitted []T
	for _, systemEvent := range w.emitted {
		if typed, ok := systemEvent.(T); ok {
			emitted = append(emitted, typed)
		}
	}
	return emitted
}

// ShardCommands returns the commands of type T that the last step sent with SendToShard, in order.
func (w *TestWorld) ShardCommands[T Command]() []ShardCommand[T] {
	var sent []ShardCommand[T]
	for _, cmd := range w.shardCommands {
		payload, ok := cmd.Payload.(T)
		if !ok {
			continue
		}
		sent = append(sent, ShardCommand[T]{
			To: OtherWorld{
				Region:       cmd.Address.GetRegion(),
				Organization: cmd.Address.GetOrganization(),
				Project:      cmd.Address.GetProject(),
				ShardID:      cmd.Address.GetServiceId(),
			},
			Payload: payload,
		})
	}
	return sent
}

// runStep resets the outputs and runs one step at the test clock. The system event tap is on only
// while run executes, so events the test emits between steps count as inputs, not outputs.
func (w *TestWorld) runStep(run func()) {
	w.events, w.shardCommands, w.emitted = nil, nil, nil
	w.step(w.now(), func() {
		w.systemEventTap = func(systemEvent ecs.SystemEvent) { w.emitted = append(w.emitted, systemEvent) }
		defer func() { w.systemEventTap = nil }()
		run()
	})
}

// now is the test clock: one second per tick, starting at the Unix epoch.
func (w *TestWorld) now() time.Time {
	return time.Unix(int64(w.currentTick.height), 0) //nolint:gosec // tick height stays far below int64 max
}
