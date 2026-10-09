package cardinal_test

import (
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestWorld_RunSystemRunsOnlyThatSystem(t *testing.T) {
	t.Parallel()

	w := cardinal.NewTestWorld(t, func(w *cardinal.World) {
		w.RegisterComponent[testutils.ComponentA]()
		w.RegisterComponent[testutils.ComponentB]()
		w.RegisterSystem(&markA{})
		w.RegisterSystem(&markB{})
	})
	entity := w.Create[marked]()

	w.RunSystem(&markA{})

	assert.Equal(t, testutils.ComponentA{X: 1}, entity.Get[testutils.ComponentA]())
	assert.Equal(t, testutils.ComponentB{}, entity.Get[testutils.ComponentB]())
}

func TestTestWorld_CommandReachesOnlyTheNextStep(t *testing.T) {
	t.Parallel()

	w := cardinal.NewTestWorld(t, func(w *cardinal.World) {
		w.RegisterCommand[testutils.SimpleCommand]()
	})
	reader := &readCommands{}

	w.Command("alice", testutils.SimpleCommand{Value: 7})
	w.RunSystem(reader)
	require.Len(t, reader.got, 1)
	assert.Equal(t, testutils.SimpleCommand{Value: 7}, reader.got[0].Payload)
	player, ok := reader.got[0].Player()
	assert.True(t, ok)
	assert.Equal(t, "alice", player)

	w.RunSystem(reader)
	assert.Empty(t, reader.got)
}

func TestTestWorld_EventsAreTheLastStepsOutput(t *testing.T) {
	t.Parallel()

	w := cardinal.NewTestWorld(t, func(w *cardinal.World) {
		w.RegisterEvent[testutils.SimpleEvent]()
		w.RegisterEvent[testutils.AnotherEvent]()
	})

	w.RunSystem(&sendEvents{})
	assert.Equal(t, []cardinal.Sent[testutils.SimpleEvent]{
		{Recipient: "", Payload: testutils.SimpleEvent{Value: 1}},
		{Recipient: "alice", Payload: testutils.SimpleEvent{Value: 2}},
	}, w.Events[testutils.SimpleEvent]())
	assert.Equal(t, []cardinal.Sent[testutils.AnotherEvent]{
		{Recipient: "bob", Payload: testutils.AnotherEvent{Data: "hi"}},
	}, w.Events[testutils.AnotherEvent]())

	w.RunSystem(&noop{})
	assert.Empty(t, w.Events[testutils.SimpleEvent]())
	assert.Empty(t, w.Events[testutils.AnotherEvent]())
}

func TestTestWorld_EncodesOutputsAsPublishingDoes(t *testing.T) {
	t.Parallel()

	lobby := cardinal.OtherWorld{Region: "us", Organization: "org", Project: "proj", ShardID: "lobby"}
	for _, tc := range []struct {
		name string
		send func(w *cardinal.World)
	}{
		{"event", func(w *cardinal.World) { w.Broadcast(unencodable{}) }},
		{"shard command", func(w *cardinal.World) { w.SendToShard(lobby, unencodable{}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := cardinal.NewTestWorld(t, func(w *cardinal.World) { w.RegisterEvent[unencodable]() })

			assert.PanicsWithValue(t, "unencodable: invalid UTF-8", func() { w.RunSystem(systemFunc(tc.send)) })
		})
	}
}

func TestTestWorld_OutputsAreWhatAReceiverGets(t *testing.T) {
	t.Parallel()

	lobby := cardinal.OtherWorld{Region: "us", Organization: "org", Project: "proj", ShardID: "lobby"}
	w := cardinal.NewTestWorld(t, func(w *cardinal.World) { w.RegisterEvent[list]() })

	values := []int{1}
	w.RunSystem(systemFunc(func(w *cardinal.World) {
		w.Broadcast(list{Values: values})
		w.SendToShard(lobby, list{Values: values})
	}))
	values[0] = 2

	assert.Equal(t, []cardinal.Sent[list]{{Payload: list{Values: []int{1}}}}, w.Events[list]())
	assert.Equal(t, []cardinal.ShardCommand[list]{{To: lobby, Payload: list{Values: []int{1}}}},
		w.ShardCommands[list]())
}

func TestTestWorld_ShardCommandsAreTheLastStepsOutput(t *testing.T) {
	t.Parallel()

	lobby := cardinal.OtherWorld{Region: "us", Organization: "org", Project: "proj", ShardID: "lobby"}
	w := cardinal.NewTestWorld(t, func(*cardinal.World) {})

	w.RunSystem(&sendToShard{to: lobby})
	assert.Equal(t, []cardinal.ShardCommand[testutils.SimpleCommand]{
		{To: lobby, Payload: testutils.SimpleCommand{Value: 3}},
	}, w.ShardCommands[testutils.SimpleCommand]())

	w.RunSystem(&noop{})
	assert.Empty(t, w.ShardCommands[testutils.SimpleCommand]())
}

func TestTestWorld_SystemEvents(t *testing.T) {
	t.Parallel()

	w := cardinal.NewTestWorld(t, func(w *cardinal.World) {
		w.RegisterSystemEvent[testutils.SimpleSystemEvent]()
		w.RegisterSystemEvent[hit]()
	})
	armor := &armor{}

	// Emitted before the step: an input the system reads, not an output of the step.
	w.EmitSystemEvent(testutils.SimpleSystemEvent{Value: 5})
	w.RunSystem(armor)

	assert.Equal(t, []testutils.SimpleSystemEvent{{Value: 5}}, armor.seen)
	assert.Equal(t, []hit{{Damage: 10}}, w.Emitted[hit]())
	assert.Empty(t, w.Emitted[testutils.SimpleSystemEvent]())
	// The step clears system events, as the end of a tick does.
	assert.Empty(t, slices.Collect(w.SystemEvents[testutils.SimpleSystemEvent]()))
	assert.Empty(t, slices.Collect(w.SystemEvents[hit]()))
}

func TestTestWorld_TickRunsHooksInOrderOnTheTestClock(t *testing.T) {
	t.Parallel()

	var log []observation
	w := cardinal.NewTestWorld(t, func(w *cardinal.World) {
		// Registered out of hook order: Tick orders by hook, not by registration.
		w.RegisterSystem(&observe{hook: "post", log: &log}, cardinal.WithHook(cardinal.PostUpdate))
		w.RegisterSystem(&observe{hook: "update", log: &log})
		w.RegisterSystem(&observe{hook: "pre", log: &log}, cardinal.WithHook(cardinal.PreUpdate))
		w.RegisterSystem(&observe{hook: "init", log: &log}, cardinal.WithHook(cardinal.Init))
	})
	for range 3 {
		w.Tick()
	}

	assert.Equal(t, []observation{
		{Hook: "init", Height: 0, Time: time.Unix(0, 0)},
		{Hook: "pre", Height: 0, Time: time.Unix(0, 0)},
		{Hook: "update", Height: 0, Time: time.Unix(0, 0)},
		{Hook: "post", Height: 0, Time: time.Unix(0, 0)},
		{Hook: "pre", Height: 1, Time: time.Unix(1, 0)},
		{Hook: "update", Height: 1, Time: time.Unix(1, 0)},
		{Hook: "post", Height: 1, Time: time.Unix(1, 0)},
		{Hook: "pre", Height: 2, Time: time.Unix(2, 0)},
		{Hook: "update", Height: 2, Time: time.Unix(2, 0)},
		{Hook: "post", Height: 2, Time: time.Unix(2, 0)},
	}, log)
}

// NewWorld would read these and dial the unreachable NATS URL for JetStream snapshot storage.
// Not parallel: t.Setenv changes the process environment.
func TestTestWorld_IgnoresEnvironment(t *testing.T) {
	t.Setenv("CARDINAL_SNAPSHOT_STORAGE_TYPE", "JETSTREAM")
	t.Setenv("NATS_URL", "nats://127.0.0.1:1")

	w := cardinal.NewTestWorld(t, func(*cardinal.World) {})
	w.Tick()

	assert.Equal(t, uint64(1), w.TickHeight())
}

func TestTestWorld_Misuse(t *testing.T) {
	t.Parallel()

	t.Run("unregistered command", func(t *testing.T) {
		t.Parallel()
		tb := &fakeTB{}
		w := cardinal.NewTestWorld(tb, func(*cardinal.World) {})

		failure := tb.run(func() { w.Command("alice", testutils.SimpleCommand{Value: 1}) })

		assert.Contains(t, failure, "unregistered command: simple_command")
	})

	t.Run("StartGame", func(t *testing.T) {
		t.Parallel()
		tb := &fakeTB{}
		w := cardinal.NewTestWorld(tb, func(*cardinal.World) {})

		failure := tb.run(w.StartGame)

		assert.Contains(t, failure, "StartGame called on a TestWorld")
	})
}

func TestRequireDeterministic_PassesAScriptThatReplays(t *testing.T) {
	t.Parallel()

	lobby := cardinal.OtherWorld{Region: "us", Organization: "org", Project: "proj", ShardID: "lobby"}
	cardinal.RequireDeterministic(t, func(w *cardinal.World) {
		w.RegisterComponent[testutils.ComponentA]()
		w.RegisterComponent[testutils.ComponentB]()
		w.RegisterCommand[testutils.SimpleCommand]()
		w.RegisterEvent[testutils.SimpleEvent]()
		w.RegisterEvent[testutils.AnotherEvent]()
		w.RegisterSystemEvent[testutils.SimpleSystemEvent]()
		w.RegisterSystemEvent[hit]()
		w.RegisterSystem(&markA{})
		w.RegisterSystem(&markB{}, cardinal.WithHook(cardinal.PostUpdate))
	}, func(w *cardinal.TestWorld) {
		w.Create[marked]()
		w.Command("alice", testutils.SimpleCommand{Value: 7})
		w.RunSystem(&readCommands{})
		w.RunSystem(&sendEvents{})
		w.RunSystem(&sendToShard{to: lobby})
		w.EmitSystemEvent(testutils.SimpleSystemEvent{Value: 5})
		w.RunSystem(&armor{})
		w.Tick()
	})
}

// Each system reads how many worlds setup has built, which differs between the two runs the way
// package-level state would. It runs as step 3, after two steps that agree.
func TestRequireDeterministic_NamesTheFirstStepThatDiffers(t *testing.T) {
	t.Parallel()

	lobby := cardinal.OtherWorld{Region: "us", Organization: "org", Project: "proj", ShardID: "lobby"}
	tests := []struct {
		differs string
		system  func(w *cardinal.World, built int)
	}{
		{"world state", func(w *cardinal.World, built int) {
			for entity := range w.Contains[marked]().Iter() {
				entity.Set(testutils.ComponentA{X: float64(built)})
			}
		}},
		{"events", func(w *cardinal.World, built int) { w.Broadcast(testutils.SimpleEvent{Value: built}) }},
		{"shard commands", func(w *cardinal.World, built int) {
			w.SendToShard(lobby, testutils.SimpleCommand{Value: built})
		}},
		{"emitted system events", func(w *cardinal.World, built int) { w.EmitSystemEvent(hit{Damage: built}) }},
	}
	for _, tt := range tests {
		t.Run(tt.differs, func(t *testing.T) {
			t.Parallel()
			built := 0
			setup := func(w *cardinal.World) {
				built++
				w.RegisterComponent[testutils.ComponentA]()
				w.RegisterComponent[testutils.ComponentB]()
				w.RegisterEvent[testutils.SimpleEvent]()
				w.RegisterSystemEvent[hit]()
			}
			tb := &fakeTB{}

			failure := tb.run(func() {
				cardinal.RequireDeterministic(tb, setup, func(w *cardinal.TestWorld) {
					w.Create[marked]()
					w.RunSystem(&noop{})
					w.RunSystem(&noop{})
					w.RunSystem(systemFunc(func(w *cardinal.World) { tt.system(w, built) }))
				})
			})

			assert.Equal(t, "cardinal: RequireDeterministic: step 3 differs between runs in "+tt.differs, failure)
		})
	}
}

func TestRequireDeterministic_FailsWhenTheRunsTakeDifferentSteps(t *testing.T) {
	t.Parallel()

	built := 0
	tb := &fakeTB{}

	failure := tb.run(func() {
		cardinal.RequireDeterministic(tb, func(*cardinal.World) { built++ }, func(w *cardinal.TestWorld) {
			for range built {
				w.RunSystem(&noop{})
			}
		})
	})

	assert.Equal(t, "cardinal: RequireDeterministic: the first run made 1 steps, the second 2", failure)
}

// World.Tick ticks are not steps, so a difference they cause shows up only when the script returns.
func TestRequireDeterministic_ComparesTheWorldWhenTheScriptReturns(t *testing.T) {
	t.Parallel()

	built := 0
	setup := func(w *cardinal.World) {
		built++
		w.RegisterComponent[testutils.ComponentA]()
		w.RegisterComponent[testutils.ComponentB]()
		w.RegisterSystem(systemFunc(func(w *cardinal.World) {
			for entity := range w.Contains[marked]().Iter() {
				entity.Set(testutils.ComponentA{X: float64(built)})
			}
		}))
	}
	tb := &fakeTB{}

	failure := tb.run(func() {
		cardinal.RequireDeterministic(tb, setup, func(w *cardinal.TestWorld) {
			w.Create[marked]()
			w.World.Tick(time.UnixMilli(1))
		})
	})

	assert.Equal(t, "cardinal: RequireDeterministic: the runs differ in world state when the script returns", failure)
}

// -------------------------------------------------------------------------------------------------
// Fixtures
// -------------------------------------------------------------------------------------------------

// marked is an archetype that markA and markB write to.
type marked struct {
	A testutils.ComponentA
	B testutils.ComponentB
}

type markA struct{}

func (*markA) Run(w *cardinal.World) {
	for entity := range w.Contains[marked]().Iter() {
		entity.Set(testutils.ComponentA{X: 1})
	}
}

type markB struct{}

func (*markB) Run(w *cardinal.World) {
	for entity := range w.Contains[marked]().Iter() {
		entity.Set(testutils.ComponentB{ID: 1})
	}
}

type readCommands struct {
	got []cardinal.CommandContext[testutils.SimpleCommand]
}

func (s *readCommands) Run(w *cardinal.World) {
	s.got = slices.Collect(w.Commands[testutils.SimpleCommand]())
}

type sendEvents struct{}

func (*sendEvents) Run(w *cardinal.World) {
	w.Broadcast(testutils.SimpleEvent{Value: 1})
	w.SendTo("bob", testutils.AnotherEvent{Data: "hi"})
	w.SendTo("alice", testutils.SimpleEvent{Value: 2})
}

type sendToShard struct {
	to cardinal.OtherWorld
}

func (s *sendToShard) Run(w *cardinal.World) {
	w.SendToShard(s.to, testutils.SimpleCommand{Value: 3})
}

type noop struct{}

func (*noop) Run(*cardinal.World) {}

// systemFunc adapts a function to a System.
type systemFunc func(w *cardinal.World)

func (f systemFunc) Run(w *cardinal.World) { f(w) }

// armor records each SimpleSystemEvent it receives and emits a hit of twice its value.
type armor struct {
	seen []testutils.SimpleSystemEvent
}

func (s *armor) Run(w *cardinal.World) {
	for evt := range w.SystemEvents[testutils.SimpleSystemEvent]() {
		s.seen = append(s.seen, evt)
		w.EmitSystemEvent(hit{Damage: evt.Value * 2})
	}
}

// hit is a system event with a hand-written varint codec, like testutils.SimpleEvent.
type hit struct {
	Damage int
}

func (hit) Name() string                 { return "hit" }
func (h hit) SizeWire() int              { return len(h.MarshalWire()) }
func (h hit) AppendWire(b []byte) []byte { return append(b, h.MarshalWire()...) }
func (h hit) MarshalWire() []byte        { return binary.AppendVarint(nil, int64(h.Damage)) }
func (hit) UnmarshalWire(b []byte) (any, error) {
	v, n := binary.Varint(b)
	if n <= 0 {
		return hit{}, errors.New("hit: malformed wire bytes")
	}
	return hit{Damage: int(v)}, nil
}

// unencodable panics when encoded, like a generated codec given a string that is not valid UTF-8.
type unencodable struct{}

func (unencodable) Name() string                      { return "unencodable" }
func (unencodable) SizeWire() int                     { panic("unencodable: invalid UTF-8") }
func (unencodable) AppendWire(b []byte) []byte        { panic("unencodable: invalid UTF-8") }
func (unencodable) UnmarshalWire([]byte) (any, error) { return unencodable{}, nil }

// list is an event and command with a slice field and a hand-written varint codec.
type list struct {
	Values []int
}

func (list) Name() string                 { return "list" }
func (l list) SizeWire() int              { return len(l.MarshalWire()) }
func (l list) AppendWire(b []byte) []byte { return append(b, l.MarshalWire()...) }

func (l list) MarshalWire() []byte {
	var b []byte
	for _, v := range l.Values {
		b = binary.AppendVarint(b, int64(v))
	}
	return b
}

func (list) UnmarshalWire(b []byte) (any, error) {
	var l list
	for len(b) > 0 {
		v, n := binary.Varint(b)
		if n <= 0 {
			return list{}, errors.New("list: malformed wire bytes")
		}
		l.Values = append(l.Values, int(v))
		b = b[n:]
	}
	return l, nil
}

type observation struct {
	Hook   string
	Height uint64
	Time   time.Time
}

// observe appends its hook, the tick height and the timestamp to log each time it runs.
type observe struct {
	hook string
	log  *[]observation
}

func (s *observe) Run(w *cardinal.World) {
	*s.log = append(*s.log, observation{Hook: s.hook, Height: w.TickHeight(), Time: w.Timestamp()})
}

// fakeTB records a fatal failure instead of failing the test that owns it. Methods TestWorld does
// not call are left to the nil embedded interface and panic if reached.
type fakeTB struct {
	testing.TB

	failure string
}

func (*fakeTB) Helper()             {}
func (*fakeTB) Log(...any)          {}
func (*fakeTB) Logf(string, ...any) {}

func (f *fakeTB) Fatal(args ...any) {
	f.failure = fmt.Sprint(args...)
	runtime.Goexit()
}

func (f *fakeTB) Fatalf(format string, args ...any) {
	f.failure = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

// run calls fn on its own goroutine, because Fatal ends the calling goroutine as [testing.T]'s does,
// and returns the recorded failure.
func (f *fakeTB) run(fn func()) string {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	<-done
	return f.failure
}
