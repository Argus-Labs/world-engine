package cardinal

import (
	"context"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/kernel"
	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hookAudience is an audience a test supplies as a function.
type hookAudience func(player string) bool

func (a hookAudience) Admits(player string, _ any) bool { return a(player) }

// hookSystem is a system a test supplies as a function.
type hookSystem func(w *World)

func (s hookSystem) Run(w *World) { s(w) }

// newHookWorld is a world that publishes through the service to one stream, alice's, and runs
// the Emit hook the test supplies.
func newHookWorld(t *testing.T, emit func(*kernel.Emit)) (*TestWorld, *streamSubscriber) {
	t.Helper()
	w := NewTestWorld(t, func(w *World) {
		w.RegisterEvent[hookBenchNote]()
		w.hooks = kernel.New(w.world)
		w.hooks.Register(kernel.Hooks{Name: "probe", Emit: emit})
	})
	w.World.events.RegisterHandler(event.KindDefault, w.service.publishDefaultEvent)
	ctx, cancel := context.WithCancelCause(t.Context())
	sub := &streamSubscriber{
		ctx: ctx, cancel: cancel, events: map[string]struct{}{}, queueReady: make(chan struct{}, 1),
	}
	addSubscriptions(sub, []*cardinalv1.EventSubscription{{Events: []string{"hook_bench_note"}}})
	w.service.addSubscriber("alice", sub)
	return w, sub
}

// An audience runs game code. The service asks it after releasing its lock, so that code can
// take the lock itself and a slow audience does not hold up streams that subscribe or leave.
func TestHook_AudienceIsAskedWithoutTheServiceLock(t *testing.T) {
	var w *TestWorld
	var lockWasFree []bool
	w, sub := newHookWorld(t, func(e *kernel.Emit) {
		e.Narrow(hookAudience(func(string) bool {
			free := w.service.mu.TryLock()
			if free {
				w.service.mu.Unlock()
			}
			lockWasFree = append(lockWasFree, free)
			return true
		}))
	})

	w.RunSystem(hookSystem(func(w *World) { w.Broadcast(hookBenchNote{N: 1}) }))

	assert.Equal(t, []bool{true}, lockWasFree)
	assert.Len(t, sub.queue, 1, "the admitted stream receives the event")
}

// Dispatch only publishes. An audience that writes to the world stops the tick, as a hook does.
func TestHook_AudienceCannotWriteToTheWorld(t *testing.T) {
	var w *TestWorld
	w, _ = newHookWorld(t, func(e *kernel.Emit) {
		e.Narrow(hookAudience(func(string) bool {
			w.world.Create()
			return true
		}))
	})

	assert.PanicsWithValue(t,
		"cardinal: event dispatch must not change the world, and it called Create",
		func() { w.RunSystem(hookSystem(func(w *World) { w.Broadcast(hookBenchNote{N: 1}) })) })
}

// The predicate a game gives EnableSync runs inside sync's hooks, so it may read the world and
// may not change it. The panic names the hook it ran in.
func TestHook_SyncPredicateCannotWriteToTheWorld(t *testing.T) {
	var w *TestWorld
	w = NewTestWorld(t, func(world *World) {
		world.RegisterComponent[syncHealth]()
		world.RegisterComponent[syncPrivate]()
		world.EnableSync(func(string) bool {
			w.Create[syncAvatar]()
			return true
		})
	})
	ctx, cancel := context.WithCancelCause(t.Context())
	sub := &streamSubscriber{
		ctx: ctx, cancel: cancel, events: map[string]struct{}{}, queueReady: make(chan struct{}, 1),
	}
	addSubscriptions(sub, []*cardinalv1.EventSubscription{{Events: []string{syncstate.EventName}}})
	w.service.addSubscriber("alice", sub)

	var panicked *kernel.PanicError
	func() {
		defer func() { panicked, _ = recover().(*kernel.PanicError) }()
		w.Tick()
	}()

	require.NotNil(t, panicked, "the tick stops")
	assert.Equal(t, "sync's Commit hook", panicked.Hook)
	assert.Equal(t, "cardinal: sync's Commit hook must not change the world, and it called Create", panicked.Value)
}

// Another registrant's audience and sync's both have to admit a reader.
func TestHook_AudiencesNarrowTogetherWithSync(t *testing.T) {
	f := newAccessFixture(t, "alice", "bob", "carol")
	f.hooks.Register(kernel.Hooks{Name: "nearby", Emit: func(e *kernel.Emit) {
		e.Narrow(hookAudience(func(player string) bool { return player == "bob" || player == "carol" }))
	}})
	avatar := f.Create[syncAvatar]()
	avatar.Set(syncstate.Viewers{Players: players("alice", "bob")})
	f.Tick()
	for _, player := range []string{"alice", "bob", "carol"} {
		f.received(player)
	}

	f.during(func(w *World) { w.Broadcast(accessSpawn{Entity: uint32(avatar.ID())}) })

	assert.NotContains(t, f.received("alice"), "access_spawn", "sync admits alice, the other audience does not")
	assert.Contains(t, f.received("bob"), "access_spawn", "both admit bob")
	assert.NotContains(t, f.received("carol"), "access_spawn", "the other audience admits carol, sync does not")
}
