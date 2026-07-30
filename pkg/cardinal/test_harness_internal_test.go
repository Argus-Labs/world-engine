package cardinal

import (
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/shamaton/msgpack/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type harnessValueComponent struct {
	Values   []int
	Personas []string
}

func (harnessValueComponent) Name() string {
	return "harness_value"
}

func (c harnessValueComponent) MarshalWire() ([]byte, error) { return msgpack.Marshal(c) }
func (harnessValueComponent) UnmarshalWire(data []byte) (any, error) {
	return unmarshalHarnessValue[harnessValueComponent](data)
}

type harnessExtraComponent struct {
	Label string
}

func (harnessExtraComponent) Name() string {
	return "harness_extra"
}

func (c harnessExtraComponent) MarshalWire() ([]byte, error) { return msgpack.Marshal(c) }
func (harnessExtraComponent) UnmarshalWire(data []byte) (any, error) {
	return unmarshalHarnessValue[harnessExtraComponent](data)
}

type harnessEvent struct {
	Values  []int
	Persona string
}

func (harnessEvent) Name() string {
	return "harness_event"
}

func (e harnessEvent) MarshalWire() ([]byte, error) { return msgpack.Marshal(e) }
func (harnessEvent) UnmarshalWire(data []byte) (any, error) {
	return unmarshalHarnessValue[harnessEvent](data)
}

type harnessSystemEvent struct {
	Value int
}

func (harnessSystemEvent) Name() string {
	return "harness_system_event"
}

func (e harnessSystemEvent) MarshalWire() ([]byte, error) { return msgpack.Marshal(e) }
func (harnessSystemEvent) UnmarshalWire(data []byte) (any, error) {
	return unmarshalHarnessValue[harnessSystemEvent](data)
}

func unmarshalHarnessValue[T any](data []byte) (any, error) {
	var value T
	if err := msgpack.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	return value, nil
}

type harnessCommandState struct {
	BaseSystemState
	Commands WithCommand[testutils.SimpleCommand]
	Entities Contains[struct {
		Value Ref[harnessValueComponent]
	}]
	Events WithEvent[harnessEvent]
}

func harnessCommandSystem(state *harnessCommandState) {
	for cmd := range state.Commands.Iter() {
		for _, entity := range state.Entities.Iter() {
			value := entity.Value.Get()
			value.Values = append(value.Values, cmd.Payload.Value)
			value.Personas = append(value.Personas, cmd.Persona)
			entity.Value.Set(value)
			state.Events.Emit(harnessEvent{
				Values:  value.Values,
				Persona: cmd.Persona,
			})
		}
	}
}

type harnessSystemEventState struct {
	BaseSystemState
	Events   WithSystemEventReceiver[harnessSystemEvent]
	Entities Contains[struct {
		Value Ref[harnessValueComponent]
	}]
}

func harnessSystemEventSystem(state *harnessSystemEventState) {
	for systemEvent := range state.Events.Iter() {
		for _, entity := range state.Entities.Iter() {
			value := entity.Value.Get()
			value.Values = append(value.Values, systemEvent.Value)
			entity.Value.Set(value)
		}
	}
}

var harnessDestination = OtherWorld{
	Region:       "test-region",
	Organization: "test-org",
	Project:      "test-project",
	ShardID:      "destination",
}

type harnessInterShardState struct {
	BaseSystemState
	Commands WithCommand[testutils.SimpleCommand]
}

func harnessInterShardSystem(state *harnessInterShardState) {
	for cmd := range state.Commands.Iter() {
		state.SendToShard(harnessDestination, cmd.Payload)
	}
}

type harnessRegistrationState struct {
	BaseSystemState
	Values Contains[struct {
		Value Ref[harnessValueComponent]
	}]
	Extras Contains[struct {
		Extra Ref[harnessExtraComponent]
	}]
}

func harnessRegistrationSystem(*harnessRegistrationState) {}

func TestTestHarness_FixturesCommandsEventsAndSystemEvents(t *testing.T) {
	start := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC)
	harness := NewTestHarness(t, TestHarnessOptions{
		TickRate:  20,
		StartTime: start,
	}, func(world *World) {
		RegisterSystem(world, harnessRegistrationSystem, WithHook(PreUpdate))
		RegisterSystem(world, harnessCommandSystem)
		RegisterSystem(world, harnessInterShardSystem)
		RegisterSystem(world, harnessSystemEventSystem, WithHook(PostUpdate))
	})

	entity := harness.Spawn(harnessValueComponent{})
	require.True(t, HasComponent[harnessValueComponent](harness, entity))
	require.False(t, HasComponent[harnessExtraComponent](harness, entity))

	SetComponent(harness, entity, harnessExtraComponent{Label: "fixture"})
	require.True(t, HasComponent[harnessExtraComponent](harness, entity))
	assert.Equal(t, "fixture", GetComponent[harnessExtraComponent](harness, entity).Label)

	InjectSystemEvent(harness, harnessSystemEvent{Value: 4})
	InjectSystemEvent(harness, harnessSystemEvent{Value: 5})
	tick0 := harness.Step(
		TestCommand{Persona: "alice", Payload: testutils.SimpleCommand{Value: 10}},
		TestCommand{Persona: "bob", Payload: testutils.SimpleCommand{Value: 20}},
	)

	assert.Equal(t, uint64(0), tick0.Height)
	assert.Equal(t, start, tick0.Timestamp)
	require.Len(t, tick0.Emissions, 4)
	assert.Equal(t, []TestEmissionKind{
		TestEmissionEvent,
		TestEmissionEvent,
		TestEmissionInterShardCommand,
		TestEmissionInterShardCommand,
	}, []TestEmissionKind{
		tick0.Emissions[0].Kind,
		tick0.Emissions[1].Kind,
		tick0.Emissions[2].Kind,
		tick0.Emissions[3].Kind,
	})

	events := EventsOf[harnessEvent](tick0)
	require.Len(t, events, 2)
	assert.Equal(t, harnessEvent{Values: []int{10}, Persona: "alice"}, events[0])
	assert.Equal(t, harnessEvent{Values: []int{10, 20}, Persona: "bob"}, events[1])

	destination := micro.String(micro.GetAddress(
		harnessDestination.Region,
		micro.RealmWorld,
		harnessDestination.Organization,
		harnessDestination.Project,
		harnessDestination.ShardID,
	))
	for i, emission := range tick0.Emissions[2:] {
		assert.Equal(t, TestEmissionInterShardCommand, emission.Kind)
		assert.Equal(t, "simple_command", emission.Name)
		assert.Equal(t, destination, emission.Destination)
		assert.NotEmpty(t, emission.Persona)
		assert.Equal(t, testutils.SimpleCommand{Value: 10 + i*10}, emission.Payload)
	}

	value := GetComponent[harnessValueComponent](harness, entity)
	assert.Equal(t, []int{10, 20, 4, 5}, value.Values)
	assert.Equal(t, []string{"alice", "bob"}, value.Personas)

	tick1 := harness.Step(TestCommand{Persona: "carol", Payload: testutils.SimpleCommand{Value: 30}})
	assert.Equal(t, uint64(1), tick1.Height)
	assert.Equal(t, start.Add(50*time.Millisecond), tick1.Timestamp)
	assert.Equal(t, []int{10, 20, 4, 5, 30}, GetComponent[harnessValueComponent](harness, entity).Values)

	// Captured event payloads are detached from later component mutations.
	assert.Equal(t, []int{10}, events[0].Values)
	assert.Equal(t, []int{10, 20}, events[1].Values)

	harness.Step()
	assert.Equal(t, []int{10, 20, 4, 5, 30}, GetComponent[harnessValueComponent](harness, entity).Values,
		"commands and system events must be visible for one tick only")

	require.True(t, harness.Destroy(entity))
	require.False(t, harness.Destroy(entity))
}

var harnessPhaseTrace *[]string

type harnessInitState struct{ BaseSystemState }
type harnessPreState struct{ BaseSystemState }
type harnessUpdateAState struct{ BaseSystemState }
type harnessUpdateBState struct{ BaseSystemState }
type harnessPostState struct{ BaseSystemState }

func harnessInitSystem(*harnessInitState) { *harnessPhaseTrace = append(*harnessPhaseTrace, "init") }
func harnessPreSystem(*harnessPreState)   { *harnessPhaseTrace = append(*harnessPhaseTrace, "pre") }
func harnessUpdateASystem(*harnessUpdateAState) {
	*harnessPhaseTrace = append(*harnessPhaseTrace, "update-a")
}
func harnessUpdateBSystem(*harnessUpdateBState) {
	*harnessPhaseTrace = append(*harnessPhaseTrace, "update-b")
}
func harnessPostSystem(*harnessPostState) { *harnessPhaseTrace = append(*harnessPhaseTrace, "post") }

func TestTestHarness_InitPhasesScheduleAndTiming(t *testing.T) {
	trace := make([]string, 0)
	harnessPhaseTrace = &trace
	t.Cleanup(func() {
		harnessPhaseTrace = nil
	})

	start := time.Unix(1_000, 123).UTC()
	harness := NewTestHarness(t, TestHarnessOptions{
		TickRate:  4,
		StartTime: start,
	}, func(world *World) {
		RegisterSystem(world, harnessUpdateASystem)
		RegisterSystem(world, harnessPostSystem, WithHook(PostUpdate))
		RegisterSystem(world, harnessInitSystem, WithHook(Init))
		RegisterSystem(world, harnessPreSystem, WithHook(PreUpdate))
		RegisterSystem(world, harnessUpdateBSystem)
	})

	assert.Equal(t, []string{"init"}, trace)
	tick0 := harness.Step()
	tick1 := harness.Step()
	assert.Equal(t, []string{
		"init",
		"pre", "update-a", "update-b", "post",
		"pre", "update-a", "update-b", "post",
	}, trace)
	assert.Equal(t, TestTick{Height: 0, Timestamp: start, Emissions: []TestEmission{}}, tick0)
	assert.Equal(t, uint64(1), tick1.Height)
	assert.Equal(t, start.Add(250*time.Millisecond), tick1.Timestamp)

	schedule := harness.Schedule()
	require.Len(t, schedule, 4)
	assertScheduleContainsInOrder(t, schedule, Init, "harnessInitState")
	assertScheduleContainsInOrder(t, schedule, PreUpdate, "harnessPreState")
	assertScheduleContainsInOrder(t, schedule, Update, "harnessUpdateAState", "harnessUpdateBState")
	assertScheduleContainsInOrder(t, schedule, PostUpdate, "harnessPostState")

	schedule[0].Systems[0] = "mutated"
	fresh := harness.Schedule()
	assert.NotEqual(t, "mutated", fresh[0].Systems[0], "Schedule must return detached copies")
}

func assertScheduleContainsInOrder(t *testing.T, schedules []TestSchedule, hook SystemHook, names ...string) {
	t.Helper()
	for _, schedule := range schedules {
		if schedule.Hook != hook {
			continue
		}
		require.Len(t, schedule.Systems, len(names))
		for i, name := range names {
			assert.Contains(t, schedule.Systems[i], name,
				"system %d for hook %d: %q does not contain %q", i, hook, schedule.Systems[i], name)
		}
		return
	}
	t.Fatalf("schedule for hook %d not found", hook)
}
