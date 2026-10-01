package cardinal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/performance"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	cardinalv1connect "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
)

const (
	spanTestTickRate     = 1.0
	spanTestTickInterval = time.Duration(float64(time.Second) / spanTestTickRate)
)

type noopSpanSystem struct{}

func (noopSpanSystem) Run(*World) {}

// spanDelaySystem sleeps for a fixed duration so its span has a measurable, non-zero
// StartOffsetNs and DurationNs.
type spanDelaySystem struct {
	delay time.Duration
}

func (s *spanDelaySystem) Run(*World) {
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
}

// newSpanBugWorld creates a world with the debug/profiling module enabled at spanTestTickRate,
// so batchSize == 1 and one tick flushes a perf batch.
func newSpanBugWorld(t *testing.T) *World {
	t.Helper()
	t.Setenv("LOG_LEVEL", "disabled")

	debug := true
	w, err := NewWorld(WorldOptions{
		Region:              "span-bug",
		Organization:        "span-bug",
		Project:             "span-bug",
		ShardID:             "0",
		TickRate:            spanTestTickRate,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        1,
		Debug:               &debug,
	})
	require.NoError(t, err)
	require.NotNil(t, w.debug)
	return w
}

// drainPerfBatch receives one perf batch from ch, failing if none arrives within a deadline.
func drainPerfBatch(t *testing.T, ch <-chan performance.Batch) performance.Batch {
	t.Helper()
	select {
	case b := <-ch:
		require.Len(t, b.Ticks, 1, "TickRate=1 flushes one tick per batch")
		return b
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a perf batch")
		return performance.Batch{}
	}
}

// TestDebugSpanOffsetSimulatedTimestamp reproduces the bug where SystemSpan.StartOffsetNs is
// computed from wall-clock time instead of the tick's time frame when the tick is driven with a
// non-wall-clock (simulated) timestamp. With the bug, StartOffsetNs becomes time.Now() - epoch,
// i.e. decades of nanoseconds, instead of a small intra-tick offset.
func TestDebugSpanOffsetSimulatedTimestamp(t *testing.T) {
	w := newSpanBugWorld(t)
	w.RegisterSystem(noopSpanSystem{}, WithHook(Update))

	ch := w.debug.perf.Subscribe()
	t.Cleanup(func() { w.debug.perf.Unsubscribe(ch) })

	w.init()

	simulatedTickStart := time.Unix(0, 0)
	w.Tick(simulatedTickStart)

	batch := drainPerfBatch(t, ch)
	tl := batch.Ticks[0]
	require.Equal(t, simulatedTickStart, tl.TickStart)
	require.Len(t, tl.Spans, 1, "one registered system")
	span := tl.Spans[0]
	startOffset := span.StartTime.Sub(tl.TickStart).Nanoseconds()

	t.Logf("TickStart (simulated) = %s", tl.TickStart)
	t.Logf("span.StartTime = %s", span.StartTime)
	t.Logf("StartOffsetNs = %d", startOffset)

	// The offset is a true intra-tick elapsed time (sub-millisecond for a no-op system),
	// not the wall-clock distance from the epoch.
	assert.Less(t, startOffset, int64(spanTestTickInterval),
		"system span offset must fit within one tick interval (%v); got decades of nanoseconds",
		spanTestTickInterval)
	assert.GreaterOrEqual(t, startOffset, int64(0), "offset must be non-negative")
	// StartTime must be in the simulated tick frame, i.e. near the epoch, not near wall-clock now.
	assert.True(t, span.StartTime.Before(time.Unix(1, 0)),
		"StartTime must be anchored to the simulated tick frame (near epoch), got %s", span.StartTime)
}

// TestDebugSpanOffsetMonotonicAndDistinct registers two systems with real delays and asserts the
// second system's span begins after the first ends and both offsets are intra-tick (below the
// interval bound) and strictly increasing.
func TestDebugSpanOffsetMonotonicAndDistinct(t *testing.T) {
	w := newSpanBugWorld(t)
	w.RegisterSystem(&spanDelaySystem{delay: 2 * time.Millisecond}, WithHook(Update))
	w.RegisterSystem(&spanDelaySystem{delay: 1 * time.Millisecond}, WithHook(Update))

	ch := w.debug.perf.Subscribe()
	t.Cleanup(func() { w.debug.perf.Unsubscribe(ch) })

	w.init()

	simulatedTickStart := time.Unix(0, 0)
	w.Tick(simulatedTickStart)

	batch := drainPerfBatch(t, ch)
	tl := batch.Ticks[0]
	require.Equal(t, simulatedTickStart, tl.TickStart)
	require.Len(t, tl.Spans, 2, "two registered systems")

	offsets := make([]int64, len(tl.Spans))
	for i, span := range tl.Spans {
		offsets[i] = span.StartTime.Sub(tl.TickStart).Nanoseconds()
		t.Logf("span[%d] %s: offset=%d ns duration=%d ns",
			i, span.SystemName, offsets[i], span.EndTime.Sub(span.StartTime).Nanoseconds())
		assert.Less(t, offsets[i], int64(spanTestTickInterval),
			"span %d offset must fit within one tick interval", i)
		assert.GreaterOrEqual(t, offsets[i], int64(0), "span %d offset must be non-negative", i)
	}

	// Distinct and monotonically increasing: the second system starts after the first.
	assert.Less(t, offsets[0], offsets[1], "offsets must be strictly increasing")
	// Each duration reflects the configured sleep (>= 1 ms with scheduler slack).
	for i, span := range tl.Spans {
		dur := span.EndTime.Sub(span.StartTime).Nanoseconds()
		assert.GreaterOrEqual(t, dur, int64(time.Millisecond),
			"span %d duration must be at least one millisecond", i)
	}
}

// TestDebugSpanOffsetWallClockTimestamp is a regression guard for the production path, where the
// tick timestamp is wall-clock time. The offset must still be a small intra-tick value rather
// than a large value (zero in theory, since the wrapper anchors to wallStart == tick start time).
func TestDebugSpanOffsetWallClockTimestamp(t *testing.T) {
	w := newSpanBugWorld(t)
	w.RegisterSystem(noopSpanSystem{}, WithHook(Update))

	ch := w.debug.perf.Subscribe()
	t.Cleanup(func() { w.debug.perf.Unsubscribe(ch) })

	w.init()

	tickStart := time.Now()
	w.Tick(tickStart)

	batch := drainPerfBatch(t, ch)
	tl := batch.Ticks[0]
	require.Len(t, tl.Spans, 1, "one registered system")
	span := tl.Spans[0]
	startOffset := span.StartTime.Sub(tl.TickStart).Nanoseconds()

	t.Logf("TickStart (wall) = %s", tl.TickStart)
	t.Logf("span.StartTime = %s", span.StartTime)
	t.Logf("StartOffsetNs = %d", startOffset)

	assert.Less(t, startOffset, int64(spanTestTickInterval),
		"wall-clock path: span offset must fit within one tick interval")
	assert.GreaterOrEqual(t, startOffset, int64(0), "offset must be non-negative")
}

// TestDebugSpanE2EStreamPerfProto validates the full proto serialization path end-to-end. It
// mounts the DebugService (the same Connect handler the gRPC server mounts) on an in-process
// httptest server, calls StreamPerf over a real Connect client, ticks with a simulated
// timestamp, and asserts the serialized SystemSpan.StartOffsetNs in the PerfBatch proto is a
// small intra-tick value, not the decades-of-nanoseconds value the bug produced.
//
// The Connect client's StreamPerf method blocks until the first response message arrives, and
// the server's StreamPerf handler blocks on the perf channel until a tick flushes a batch. To
// avoid a deadlock (client waits for first message, handler waits for a tick, no tick until the
// client returns), the test starts a background ticker BEFORE opening the RPC so batches are
// already flowing when the handler subscribes.
func TestDebugSpanE2EStreamPerfProto(t *testing.T) {
	w := newSpanBugWorld(t)
	w.RegisterSystem(noopSpanSystem{}, WithHook(Update))

	w.service = newService(w, AuthModeDev, "")
	w.init()

	// Finalize the introspection catalog and mount the debug service on an httptest server,
	// exactly as production mounts it on the client-facing port.
	require.NoError(t, w.debug.finalizeCatalog())
	mux := http.NewServeMux()
	debugPath, debugHandler := cardinalv1connect.NewDebugServiceHandler(w.debug)
	mux.Handle(debugPath, debugHandler)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start ticking BEFORE opening the RPC, so batches are flowing when the StreamPerf
	// handler subscribes. Each tick uses the same simulated start timestamp so the assertion
	// on TickStart is stable regardless of which batch arrives first.
	simulatedTickStart := time.Unix(0, 0).UTC()
	go func() {
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			default:
			}
			w.Tick(simulatedTickStart)
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Open the streaming RPC while ticks are flowing.
	client := cardinalv1connect.NewDebugServiceClient(server.Client(), server.URL)
	stream, err := client.StreamPerf(ctx, connect.NewRequest(&cardinalv1.StreamPerfRequest{}))
	require.NoError(t, err, "StreamPerf open failed")

	require.True(t, stream.Receive(), "expected a PerfBatch on the stream")
	proto := stream.Msg()
	require.NoError(t, stream.Err())

	require.NotEmpty(t, proto.GetTicks(), "at least one tick per batch")
	tl := proto.GetTicks()[0]
	require.Len(t, tl.GetSpans(), 1, "one registered system")
	span := tl.GetSpans()[0]

	t.Logf("proto TickHeight = %d", tl.GetTickHeight())
	t.Logf("proto TickStart = %s", tl.GetTickStart().AsTime())
	t.Logf("proto StartOffsetNs = %d", span.GetStartOffsetNs())
	t.Logf("proto DurationNs = %d", span.GetDurationNs())

	// The serialized uint64 offset must be a small intra-tick value (< 1s), not decades.
	assert.Less(t, span.GetStartOffsetNs(), uint64(spanTestTickInterval),
		"proto StartOffsetNs must fit within one tick interval; got decades of nanoseconds (bug)")
	// DurationNs is also intra-tick for a no-op system (well under 1s).
	assert.Less(t, span.GetDurationNs(), uint64(spanTestTickInterval),
		"proto DurationNs must fit within one tick interval for a no-op system")
	assert.Equal(t, simulatedTickStart, tl.GetTickStart().AsTime(),
		"proto TickStart must be the simulated timestamp")
}
