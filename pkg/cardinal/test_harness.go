package cardinal

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/micro"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
)

const testHarnessIdentity = "test"

// Component is a component value accepted by TestHarness fixture operations.
type Component = ecs.Component

// SystemEvent is an in-process event accepted by TestHarness fixture operations.
type SystemEvent = ecs.SystemEvent

// TestHarnessOptions controls deterministic tick timing.
type TestHarnessOptions struct {
	TickRate  float64
	StartTime time.Time
}

// TestCommand is one command delivered at the start of the next Step.
type TestCommand struct {
	Persona string
	Payload Command
}

// TestEmissionKind identifies one locally captured outward delivery.
type TestEmissionKind uint8

const (
	// TestEmissionEvent is a normal event emitted to clients.
	TestEmissionEvent TestEmissionKind = iota
	// TestEmissionInterShardCommand is a command addressed to another world.
	TestEmissionInterShardCommand
)

// TestEmission is an immutable wire-equivalent copy of one outward delivery.
type TestEmission struct {
	Kind        TestEmissionKind
	Name        string
	Payload     any
	Persona     string
	Destination string
}

// TestTick describes one committed deterministic tick.
type TestTick struct {
	Height    uint64
	Timestamp time.Time
	Emissions []TestEmission
}

// TestSchedule describes registered systems for one hook.
type TestSchedule struct {
	Hook    SystemHook
	Systems []string
}

// TestHarness drives a real Cardinal world without starting remote transports.
type TestHarness struct {
	t        testing.TB
	world    *World
	options  TestHarnessOptions
	capture  testEmissionCapture
	stepping bool
}

// NewTestHarness creates and initializes a deterministic Cardinal world.
func NewTestHarness(
	t testing.TB,
	options TestHarnessOptions,
	setup func(*World),
) *TestHarness {
	t.Helper()
	if options.TickRate <= 0 {
		t.Fatalf("cardinal test harness tick rate must be greater than zero")
	}

	disabled := false
	world, err := NewWorld(WorldOptions{
		Region:              testHarnessIdentity,
		Organization:        testHarnessIdentity,
		Project:             testHarnessIdentity,
		ShardID:             "0",
		TickRate:            options.TickRate,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        math.MaxUint32,
		Debug:               &disabled,
		Pprof:               &disabled,
		AuthMode:            AuthModeDev,
	})
	if err != nil {
		t.Fatalf("create Cardinal test world: %v", err)
	}

	harness := &TestHarness{
		t:       t,
		world:   world,
		options: options,
		capture: testEmissionCapture{
			emissions: make([]TestEmission, 0),
		},
	}
	world.events.RegisterHandler(event.KindDefault, harness.capture.defaultEvent)
	world.events.RegisterHandler(event.KindInterShardCommand, harness.capture.interShardCommand)

	if setup != nil {
		setup(world)
	}
	world.initialize()
	ecs.CheckWorld(t, world.world)

	return harness
}

// Spawn creates an entity from registered component values.
func (h *TestHarness) Spawn(components ...Component) EntityID {
	h.t.Helper()
	h.requireFixtureWindow()

	eid, err := ecs.CreateWithComponents(h.world.world, components...)
	if err != nil {
		h.t.Fatalf("spawn entity: %v", err)
	}
	ecs.CheckWorld(h.t, h.world.world)
	return eid
}

// Destroy deletes an entity and all its components.
func (h *TestHarness) Destroy(eid EntityID) bool {
	h.t.Helper()
	h.requireFixtureWindow()

	destroyed := ecs.Destroy(h.world.world, eid)
	ecs.CheckWorld(h.t, h.world.world)
	return destroyed
}

// Step delivers commands and commits one deterministic Cardinal tick.
func (h *TestHarness) Step(commands ...TestCommand) TestTick {
	h.t.Helper()
	h.requireFixtureWindow()

	wireCommands := make([]*iscv1.Command, len(commands))
	for i, testCommand := range commands {
		if testCommand.Payload == nil {
			h.t.Fatalf("command %d has nil payload", i)
		}
		payload, err := testCommand.Payload.MarshalWire()
		if err != nil {
			h.t.Fatalf("encode command %d (%q): %v", i, testCommand.Payload.Name(), err)
		}
		wireCommand := &iscv1.Command{
			Name:    testCommand.Payload.Name(),
			Address: h.world.address,
			Persona: &iscv1.Persona{Id: testCommand.Persona},
			Payload: payload,
		}
		if err := h.world.validateCommand(wireCommand); err != nil {
			h.t.Fatalf("validate command %d (%q): %v", i, testCommand.Payload.Name(), err)
		}
		wireCommands[i] = wireCommand
	}

	h.stepping = true
	defer func() {
		h.stepping = false
	}()

	for i, wireCommand := range wireCommands {
		if err := h.world.enqueueCommand(wireCommand); err != nil {
			h.t.Fatalf("enqueue prevalidated command %d (%q): %v", i, wireCommand.GetName(), err)
		}
	}

	h.capture.reset()
	height := h.world.currentTick.height
	timestamp := h.options.StartTime.Add(testTickOffset(height, h.options.TickRate))
	h.world.Tick(context.Background(), timestamp)
	ecs.CheckWorld(h.t, h.world.world)

	if h.capture.err != nil {
		h.t.Fatalf("capture committed tick %d emissions: %v", height, h.capture.err)
	}

	return TestTick{
		Height:    height,
		Timestamp: timestamp,
		Emissions: h.capture.copyEmissions(),
	}
}

// Schedule returns a detached copy of the current system registration order.
func (h *TestHarness) Schedule() []TestSchedule {
	h.t.Helper()
	h.requireFixtureWindow()
	schedules := h.world.world.Schedules()
	result := make([]TestSchedule, len(schedules))
	for i, schedule := range schedules {
		systems := make([]string, len(schedule.Systems))
		for j, system := range schedule.Systems {
			systems[j] = system.Name
		}
		result[i] = TestSchedule{
			Hook:    schedule.Hook,
			Systems: systems,
		}
	}
	return result
}

// GetComponent returns one entity component or fails the test.
func GetComponent[C Component](h *TestHarness, eid EntityID) C {
	h.t.Helper()
	h.requireFixtureWindow()

	component, err := ecs.Get[C](h.world.world, eid)
	if err != nil {
		h.t.Fatalf("get component from entity %d: %v", eid, err)
	}
	return component
}

// SetComponent sets or adds one registered entity component.
func SetComponent[C Component](h *TestHarness, eid EntityID, component C) {
	h.t.Helper()
	h.requireFixtureWindow()

	if err := ecs.Set(h.world.world, eid, component); err != nil {
		h.t.Fatalf("set component on entity %d: %v", eid, err)
	}
	ecs.CheckWorld(h.t, h.world.world)
}

// HasComponent reports whether an entity has one component type.
func HasComponent[C Component](h *TestHarness, eid EntityID) bool {
	h.t.Helper()
	h.requireFixtureWindow()
	return ecs.Has[C](h.world.world, eid)
}

// InjectSystemEvent makes an already registered system event visible during the next tick.
func InjectSystemEvent[E SystemEvent](h *TestHarness, systemEvent E) {
	h.t.Helper()
	h.requireFixtureWindow()

	if err := ecs.EmitSystemEvent(h.world.world, systemEvent); err != nil {
		h.t.Fatalf("inject system event %q: %v", systemEvent.Name(), err)
	}
}

// EventsOf returns captured normal events of the requested concrete type in delivery order.
func EventsOf[E Event](tick TestTick) []E {
	events := make([]E, 0)
	for _, emission := range tick.Emissions {
		if emission.Kind != TestEmissionEvent {
			continue
		}
		payload, ok := emission.Payload.(E)
		if ok {
			events = append(events, payload)
		}
	}
	return events
}

func (h *TestHarness) requireFixtureWindow() {
	h.t.Helper()
	if h.stepping {
		h.t.Fatalf("Cardinal test fixture operations are only valid between ticks")
	}
}

func testTickOffset(height uint64, tickRate float64) time.Duration {
	return time.Duration(float64(time.Second) * float64(height) / tickRate)
}

type testEmissionCapture struct {
	emissions []TestEmission
	err       error
}

func (c *testEmissionCapture) reset() {
	c.emissions = c.emissions[:0]
	c.err = nil
}

func (c *testEmissionCapture) copyEmissions() []TestEmission {
	emissions := make([]TestEmission, len(c.emissions))
	copy(emissions, c.emissions)
	return emissions
}

func (c *testEmissionCapture) defaultEvent(evt event.Event) error {
	payload, ok := evt.Payload.(event.Payload)
	if !ok {
		return c.recordError(eris.Errorf("invalid event payload type: %T", evt.Payload))
	}

	clone, err := cloneSerializable(payload)
	if err != nil {
		return c.recordError(eris.Wrapf(err, "clone event %q", payload.Name()))
	}
	c.emissions = append(c.emissions, TestEmission{
		Kind:    TestEmissionEvent,
		Name:    payload.Name(),
		Payload: clone,
	})
	return nil
}

func (c *testEmissionCapture) interShardCommand(evt event.Event) error {
	outbound, ok := evt.Payload.(command.Command)
	if !ok {
		return c.recordError(eris.Errorf("invalid inter-shard command payload type: %T", evt.Payload))
	}
	if outbound.Address == nil {
		return c.recordError(eris.New("inter-shard command has nil destination"))
	}

	wirePayload, err := outbound.Payload.MarshalWire()
	if err != nil {
		return c.recordError(eris.Wrapf(err, "encode inter-shard command %q", outbound.Name))
	}
	clone, err := outbound.Payload.UnmarshalWire(wirePayload)
	if err != nil {
		return c.recordError(eris.Wrapf(err, "decode inter-shard command %q", outbound.Name))
	}

	c.emissions = append(c.emissions, TestEmission{
		Kind:        TestEmissionInterShardCommand,
		Name:        outbound.Payload.Name(),
		Payload:     clone,
		Persona:     outbound.Persona,
		Destination: micro.String(outbound.Address),
	})
	return nil
}

func (c *testEmissionCapture) recordError(err error) error {
	if c.err == nil {
		c.err = err
	}
	return err
}

func cloneSerializable(value schema.Serializable) (any, error) {
	wire, err := value.MarshalWire()
	if err != nil {
		return nil, err
	}
	return value.UnmarshalWire(wire)
}
