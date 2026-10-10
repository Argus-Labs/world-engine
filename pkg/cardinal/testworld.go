package cardinal

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
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
	oteltrace "go.opentelemetry.io/otel/trace"
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

	// Digests after init and after each step. Recorded only when non-nil, which only
	// RequireDeterministic sets.
	digests []stepDigest
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
	metrics, err := newWorldMetrics(0)
	if err != nil {
		t.Fatal(err)
	}
	storage := snapshot.NewNopStorage()
	w := &World{
		world:    ecs.NewWorld(),
		commands: command.NewManager(),
		events:   event.NewManager(1024),
		// SendToShard stamps the sender's address on every command, so the world needs one.
		address:         micro.GetAddress("test", micro.RealmWorld, "test", "test", "test"),
		snapshotStorage: storage,
		snapshotWriter:  snapshot.NewSyncWriter(storage, tel.GetLogger("snapshot")),
		traceCtx:        context.Background(),
		// Encode the state every step, so a component that cannot be serialized fails the step that
		// stored it rather than a production snapshot.
		options: WorldOptions{SnapshotRate: 1},
		tel:     tel,
		metrics: metrics,
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

// Command sends cmd from player the way a client does. The command is encoded, decoded by its
// queue and read by Commands[T] during the next step only. It fails the test if the command type was
// not registered in setup.
func (w *TestWorld) Command(player string, cmd Command) {
	w.tb.Helper()
	err := w.commands.Enqueue(context.Background(), &iscv1.Command{
		Name:    cmd.Name(),
		Address: w.address,
		Payload: schema.Marshal(cmd),
	}, command.PlayerSender(player))
	if err != nil {
		w.tb.Fatalf("cardinal: Command %s: %v", cmd.Name(), err)
	}
}

// RunSystem runs exactly one system as a step. The system need not be registered. System events
// emitted before the step are visible to it and are cleared when it returns, as at the end of a tick.
// It runs under a span named after the system like a registered system, without the hook attribute.
func (w *TestWorld) RunSystem(s System) {
	w.runStep(func() {
		defer w.world.ClearSystemEvents()
		name := fmt.Sprintf("%T", s)
		w.runSystem(s, name, oteltrace.WithAttributes(attrSystemName.String(name)))
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
	if w.digests != nil {
		w.digests = append(w.digests, w.digest())
	}
}

// now is the test clock: one second per tick, starting at the Unix epoch.
func (w *TestWorld) now() time.Time {
	return time.Unix(int64(w.currentTick.height), 0) //nolint:gosec // tick height stays far below int64 max
}

// RequireDeterministic runs script on two fresh worlds built by setup and fails the test at the
// first step after which they differ in world state, events, shard commands or emitted system
// events, or when they differ after script returns.
//
// Step 0 is the world after init. Step n is the nth RunSystem or Tick the script makes. Ticks the
// script runs with w.World.Tick(timestamp) are not steps, so their effect is compared only when
// script returns.
//
// The check compares two runs in one process, so it catches nondeterminism only when those runs
// differ. Package state, unseeded randomness and nanosecond clock reads almost always do. Iteration
// over a map of a few keys, or a clock read at second granularity, often does not: range over
// [slices.Sorted]([maps.Keys](m)) and read w.Timestamp() instead of [time.Now].
func RequireDeterministic(t testing.TB, setup func(w *World), script func(w *TestWorld)) {
	t.Helper()

	first, second := recordRun(t, setup, script), recordRun(t, setup, script)
	for step := range min(len(first.steps), len(second.steps)) {
		if diff := first.steps[step].diff(second.steps[step]); diff != "" {
			t.Fatalf("cardinal: RequireDeterministic: step %d differs between runs in %s", step, diff)
		}
	}
	if len(first.steps) != len(second.steps) {
		t.Fatalf("cardinal: RequireDeterministic: the first run made %d steps, the second %d",
			len(first.steps)-1, len(second.steps)-1)
	}
	if diff := first.end.diff(second.end); diff != "" {
		t.Fatalf("cardinal: RequireDeterministic: the runs differ in %s when the script returns", diff)
	}
}

// recordedRun holds one RequireDeterministic run's digests: after init and each step, and after script
// returns.
type recordedRun struct {
	steps []stepDigest
	end   stepDigest
}

// recordRun runs script on a fresh world and returns its digests.
func recordRun(t testing.TB, setup func(w *World), script func(w *TestWorld)) recordedRun {
	t.Helper()
	w := NewTestWorld(t, setup)
	w.digests = []stepDigest{w.digest()}
	script(w)
	return recordedRun{steps: w.digests, end: w.digest()}
}

// stepDigest fingerprints the world after one step: its encoded state and each kind of output.
type stepDigest struct {
	state, events, shardCommands, emitted [sha256.Size]byte
}

func (w *TestWorld) digest() stepDigest {
	var events, shardCommands, emitted []byte
	for _, evt := range w.events {
		payload, ok := evt.Payload.(schema.Serializable)
		assert.That(ok, "event payload is %T, want schema.Serializable", evt.Payload)
		events = appendPayload(appendString(events, evt.Recipient), payload)
	}
	for _, cmd := range w.shardCommands {
		shardCommands = appendPayload(appendString(shardCommands, micro.String(cmd.Address)), cmd.Payload)
	}
	for _, systemEvent := range w.emitted {
		emitted = appendPayload(emitted, systemEvent)
	}
	return stepDigest{
		state:         sha256.Sum256(w.world.EncodeState(nil)),
		events:        sha256.Sum256(events),
		shardCommands: sha256.Sum256(shardCommands),
		emitted:       sha256.Sum256(emitted),
	}
}

// diff names the parts of d that differ from other, or returns "" when none do.
func (d stepDigest) diff(other stepDigest) string {
	var parts []string
	if d.state != other.state {
		parts = append(parts, "world state")
	}
	if d.events != other.events {
		parts = append(parts, "events")
	}
	if d.shardCommands != other.shardCommands {
		parts = append(parts, "shard commands")
	}
	if d.emitted != other.emitted {
		parts = append(parts, "emitted system events")
	}
	return strings.Join(parts, ", ")
}

// appendString and appendPayload length-prefix every value, so adjacent values cannot run together.
func appendString(b []byte, s string) []byte {
	return append(binary.AppendUvarint(b, uint64(len(s))), s...)
}

func appendPayload(b []byte, p schema.Serializable) []byte {
	b = appendString(b, p.Name())
	b = binary.AppendUvarint(b, uint64(p.SizeWire())) //nolint:gosec // a size is non-negative
	return p.AppendWire(b)
}
