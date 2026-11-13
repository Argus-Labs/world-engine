package cardinal

import (
	"testing"
	"time"

	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/stretchr/testify/require"
)

// syncSeen is one frame a reader received, by the tick that sent it.
type syncSeen struct {
	at        int64  // the tick, in milliseconds after the test's start
	kind      string // baseline, delta or empty
	time      uint64 // the frame's Time
	heartbeat uint64 // a baseline's Open.Heartbeat
}

func syncSeenOf(at int64, f syncstate.Frame) syncSeen {
	seen := syncSeen{at: at, kind: syncKind(f), time: f.Time}
	if f.Open != nil {
		seen.heartbeat = f.Open.Heartbeat
	}
	return seen
}

func syncKind(f syncstate.Frame) string {
	switch {
	case f.Open != nil:
		return "baseline"
	case emptyDelta(f):
		return "empty"
	}
	return "delta"
}

// setSyncSubscribed subscribes sub to world_sync or unsubscribes it, as the service's RPCs do.
func setSyncSubscribed(w *TestWorld, sub *streamSubscriber, on bool) {
	subscriptions := []*cardinalv1.EventSubscription{{Events: []string{syncstate.EventName}}}
	w.service.mu.Lock()
	defer w.service.mu.Unlock()
	if on {
		addSubscriptions(sub, subscriptions)
	} else {
		removeSubscriptions(sub, subscriptions)
	}
}

// A client rebuilds server time by adding Times: a baseline's is its tick's Unix milliseconds, a
// delta's the milliseconds since the stream's previous frame. The sum at every frame is that
// frame's tick time, across deltas, two frames in one tick, an empty delta, and the baselines of a
// revocation and a readmission on the same reader.
func TestSyncTimesAddUpToEachFrameTick(t *testing.T) {
	admitted := true
	w, sub := newSyncFixture(t, func(string) bool { return admitted })
	e := w.Create[syncAvatar]()
	start := time.UnixMilli(1_762_950_000_123).Add(456 * time.Microsecond) // milliseconds are whole
	startMs := uint64(start.UnixMilli())
	var seen []syncSeen
	var sum uint64
	for at := int64(0); at <= 2300; at += 50 {
		switch at {
		case 50:
			e.Set(syncHealth{HP: 1})
		case 100: // two commits in one tick: an event is a publication barrier
			e.Set(syncHealth{HP: 2})
			w.Broadcast(syncHurt{Amount: 1})
			e.Set(syncHealth{HP: 3})
		case 1150: // revoked: an empty baseline on a new stream clears the view
			admitted = false
		case 1200:
			admitted = true
		case 1250:
			e.Set(syncHealth{HP: 4})
		}
		tick := start.Add(time.Duration(at) * time.Millisecond)
		w.World.Tick(tick)
		for _, f := range syncFrames(t, sub) {
			if f.Open != nil {
				sum = 0
			}
			sum += f.Time
			require.Equal(t, uint64(tick.UnixMilli()), sum, "frame %d at %d ms", f.Sequence, at)
			seen = append(seen, syncSeenOf(at, f))
		}
	}
	require.Equal(t, []syncSeen{
		{0, "baseline", startMs, 1000},
		{50, "delta", 50, 0},
		{100, "delta", 50, 0},
		{100, "delta", 0, 0},
		{1100, "empty", 1000, 0},
		{1150, "baseline", startMs + 1150, 0}, // clearing a revoked view promises no heartbeat
		{1200, "baseline", startMs + 1200, 1000},
		{1250, "delta", 50, 0},
		{2250, "empty", 1000, 0},
	}, seen)
}

// A seated reader with an open stream receives a frame at least every heartbeat of server time:
// an empty delta when it has nothing to hear, on the heartbeat after its last frame or baseline,
// and on every tick of a world that ticks slower than that. A reader told something every tick
// gets nothing more. A cancelled or closed stream gets no heartbeat.
func TestSyncHeartbeatEveryHeartbeatOfServerTime(t *testing.T) {
	revoked := false
	w, idle := newSyncFixture(t, func(player string) bool { return player != "alice" || !revoked })
	busy := reconnectSync(t, w, "bob")
	e := w.Create[syncAvatar]()
	e.Set(syncstate.Viewers{Players: players("bob")}) // alice sees nothing, so she hears nothing
	start := time.UnixMilli(1_762_950_000_000)
	var heard []syncSeen
	hp := 0
	tick := func(at int64) {
		t.Helper()
		hp++
		e.Set(syncHealth{HP: hp})
		w.World.Tick(start.Add(time.Duration(at) * time.Millisecond))
		for _, f := range syncFrames(t, idle) {
			heard = append(heard, syncSeenOf(at, f))
		}
		if busy != nil {
			frames := syncFrames(t, busy)
			require.Len(t, frames, 1, "bob at %d ms: one frame a tick", at)
			require.NotEqual(t, "empty", syncKind(frames[0]), "bob at %d ms", at)
		}
	}

	at := int64(0)
	for ; at <= 3000; at += 50 { // 20 ticks a second
		tick(at)
	}
	revoked = true // cleared at 3050 and readmitted at 3100: each baseline restarts the clock
	tick(at)
	revoked = false
	for at += 50; at <= 4200; at += 50 {
		tick(at)
	}
	for at = 5600; at <= 7100; at += 1500 { // slower than the heartbeat
		tick(at)
	}
	require.Equal(t, []syncSeen{
		{0, "baseline", 1_762_950_000_000, 1000},
		{1000, "empty", 1000, 0},
		{2000, "empty", 1000, 0},
		{3000, "empty", 1000, 0},
		{3050, "baseline", 1_762_950_003_050, 0}, // revoked: no promise, and no heartbeat follows
		{3100, "baseline", 1_762_950_003_100, 1000},
		{4100, "empty", 1000, 0},
		{5600, "empty", 1500, 0},
		{7100, "empty", 1500, 0},
	}, heard)

	idle.cancel(nil) // still subscribed and seated, but its stream is cancelled
	setSyncSubscribed(w, busy, false)
	tick(7150) // the tick that closes bob's stream still delivers its commit, before the close
	closed := busy
	busy = nil
	for at = 7200; at <= 10_000; at += 50 {
		tick(at)
	}
	require.Len(t, heard, 9, "a cancelled stream gets no heartbeat")
	require.Empty(t, syncFrames(t, closed), "a closed stream gets no heartbeat")
}
