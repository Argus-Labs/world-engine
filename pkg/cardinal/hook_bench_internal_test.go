package cardinal

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
)

type hookBenchPing struct{ N int }

func (hookBenchPing) Name() string { return "hook_bench_ping" }

type hookBenchNote struct{ N int }

func (hookBenchNote) Name() string { return "hook_bench_note" }

// hookBenchEmitter broadcasts notes, which no hook narrows, and spawns about one synced entity,
// which sync stamps and narrows to the entity's viewers.
type hookBenchEmitter struct {
	notes, spawns int
	about         *EntityID
}

func (s hookBenchEmitter) Run(w *World) {
	for i := range s.notes {
		w.Broadcast(hookBenchNote{N: i})
	}
	for range s.spawns {
		w.Broadcast(accessSpawn{Entity: uint32(*s.about)})
	}
}

// newHookBenchWorld is a world with sync on, one synced entity and readers subscribed to every
// event the emitter sends. It publishes through the service, as a running shard does, and encodes
// no snapshot after the first tick, so a tick measures the hooks and not the encoder.
func newHookBenchWorld(b *testing.B, emitter hookBenchEmitter, readers int) (*TestWorld, func()) {
	b.Helper()
	about := new(EntityID)
	emitter.about = about
	w := NewTestWorld(b, func(w *World) {
		w.RegisterComponent[syncHealth]()
		w.RegisterComponent[syncPrivate]()
		w.EnableSync(func(string) bool { return true })
		w.RegisterCommand[hookBenchPing](syncstate.FromPlayers)
		w.RegisterEvent[hookBenchNote]()
		w.RegisterEvent[accessSpawn]()
		w.RegisterSystem(emitter)
	})
	w.options.SnapshotRate = math.MaxUint32
	w.World.events.RegisterHandler(event.KindDefault, w.service.publishDefaultEvent)
	var subs []*streamSubscriber
	for i := range readers {
		ctx, cancel := context.WithCancelCause(b.Context())
		sub := &streamSubscriber{
			ctx: ctx, cancel: cancel, queueReady: make(chan struct{}, 1), events: map[string]struct{}{},
		}
		addSubscriptions(sub, []*cardinalv1.EventSubscription{{Events: []string{
			syncstate.EventName, "hook_bench_note", "access_spawn",
		}}})
		w.service.addSubscriber(fmt.Sprintf("bot-%d", i), sub)
		subs = append(subs, sub)
	}
	*about = w.Create[syncAvatar]().ID()
	drain := func() {
		for _, sub := range subs {
			sub.queue, sub.queuedBytes = nil, 0
		}
	}
	w.Tick() // the readers' baselines, and the only snapshot
	drain()
	return w, drain
}

// BenchmarkHookTick is one tick's cost of the hooks sync runs: before the first system, at each
// command and event, and around dispatch. The world holds one entity and nothing changes it.
func BenchmarkHookTick(b *testing.B) {
	b.Run("idle", func(b *testing.B) {
		w, drain := newHookBenchWorld(b, hookBenchEmitter{}, 0)
		b.ReportAllocs()
		for b.Loop() {
			w.Tick()
			drain()
		}
	})
	b.Run("commands=100", func(b *testing.B) {
		w, drain := newHookBenchWorld(b, hookBenchEmitter{}, 0)
		b.ReportAllocs()
		for b.Loop() {
			b.StopTimer()
			for i := range 100 {
				w.Command(fmt.Sprintf("bot-%d", i%10), hookBenchPing{N: i})
			}
			b.StartTimer()
			w.Tick()
			drain()
		}
	})
	b.Run("events=100", func(b *testing.B) {
		w, drain := newHookBenchWorld(b, hookBenchEmitter{notes: 100}, 0)
		b.ReportAllocs()
		for b.Loop() {
			w.Tick()
			drain()
		}
	})
	b.Run("narrowed events=20/readers=200", func(b *testing.B) {
		w, drain := newHookBenchWorld(b, hookBenchEmitter{spawns: 20}, 200)
		b.ReportAllocs()
		for b.Loop() {
			w.Tick()
			drain()
		}
	})
}

// BenchmarkHookWrite is what one write pays while sync observes the world.
func BenchmarkHookWrite(b *testing.B) {
	b.Run("Set", func(b *testing.B) {
		w, _ := newHookBenchWorld(b, hookBenchEmitter{}, 0)
		e := w.Create[syncAvatar]()
		b.ReportAllocs()
		hp := 0
		for b.Loop() {
			hp++
			e.Set(syncHealth{HP: hp, MaxHP: 100})
		}
	})
	b.Run("CreateDestroy", func(b *testing.B) {
		w, _ := newHookBenchWorld(b, hookBenchEmitter{}, 0)
		b.ReportAllocs()
		for b.Loop() {
			w.Create[syncAvatar]().Destroy()
		}
	})
}
