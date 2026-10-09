package transport

import (
	"context"
	cryptorand "crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Event stream replacement
// -------------------------------------------------------------------------------------------------
// Drives the production handler over an h2c httptest server with a real Connect gRPC client. A
// player's newest stream replaces the old one: the old stream ends Aborted ("superseded"), the new
// one receives broadcasts and SendTo events, and the old handler's cleanup never unregisters it.
// -------------------------------------------------------------------------------------------------

const testStreamIDHeader = "X-Test-Stream-Id"

func TestEventStream_ReplacementSupersedesOldStream(t *testing.T) {
	t.Parallel()
	h := newStreamHarness(t)
	simple := testutils.SimpleEvent{}.Name()
	another := testutils.AnotherEvent{}.Name()

	a := h.open(t, "player-1", "*")
	b := h.open(t, "player-1", simple)
	a.read()
	b.read()

	requireSuperseded(t, a)

	// A's handler has returned (its trailers arrived), so its deferred cleanup already ran.
	h.publish(t, testutils.AnotherEvent{Data: "b did not subscribe"}, "")
	h.publish(t, testutils.SimpleEvent{Value: 1}, "")
	h.publish(t, testutils.SimpleEvent{Value: 2}, "player-1")
	assert.Equal(t, []string{"simple_event:1", "simple_event:2"}, b.next(t, 2))

	h.subscribe(t, "player-1", another)
	h.publish(t, testutils.AnotherEvent{Data: "now subscribed"}, "player-1")
	assert.Equal(t, []string{"another_event:now subscribed"}, b.next(t, 1))
	b.requireOpen(t)
}

func TestEventStream_RapidSequentialReplacement(t *testing.T) {
	t.Parallel()
	h := newStreamHarness(t)

	const n = 25
	streams := make([]*clientStream, n)
	for i := range streams {
		streams[i] = h.open(t, "player-1", "*")
		streams[i].read()
	}

	for _, stream := range streams[:n-1] {
		requireSuperseded(t, stream)
	}
	h.publish(t, testutils.SimpleEvent{Value: 7}, "player-1")
	assert.Equal(t, []string{"simple_event:7"}, streams[n-1].next(t, 1))
}

func TestEventStream_ConcurrentReplacementLeavesOneStream(t *testing.T) {
	t.Parallel()
	h := newStreamHarness(t)

	const n = 16
	streams := make([]*clientStream, n)
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range streams {
		wg.Go(func() { streams[i], errs[i] = h.tryOpen("player-1", "*") })
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	for _, stream := range streams {
		stream.read()
	}

	h.publish(t, testutils.SimpleEvent{Value: 9}, "player-1")

	var superseded, delivered int
	for _, stream := range streams {
		select {
		case msg := <-stream.messages:
			assert.Equal(t, "simple_event:9", describe(msg))
			delivered++
		case <-stream.done:
			assert.Equal(t, connect.CodeAborted, connect.CodeOf(stream.err))
			assert.Contains(t, stream.err.Error(), "superseded")
			superseded++
		case <-time.After(5 * time.Second):
			t.Fatal("stream neither received the event nor ended")
		}
	}
	assert.Equal(t, 1, delivered)
	assert.Equal(t, n-1, superseded)
}

// Subscription RPCs racing replacements and closes either update the stream registered at that
// moment or fail with FailedPrecondition. They never panic or return any other error.
func TestEventStream_SubscriptionRPCsRaceReplacement(t *testing.T) {
	t.Parallel()
	h := newStreamHarness(t)
	simple := testutils.SimpleEvent{}.Name()
	another := testutils.AnotherEvent{}.Name()

	stop := make(chan struct{})
	var unexpected sync.Map
	var calls, accepted atomic.Int64
	var spammers sync.WaitGroup
	for i := range 4 {
		spammers.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				var err error
				if i%2 == 0 {
					err = h.trySubscribe("player-1", another)
				} else {
					err = h.tryUnsubscribe("player-1", another)
				}
				calls.Add(1)
				switch {
				case err == nil:
					accepted.Add(1)
				case connect.CodeOf(err) != connect.CodeFailedPrecondition:
					unexpected.Store(err.Error(), struct{}{})
				}
			}
		})
	}

	for i := range 40 {
		stream := h.open(t, "player-1", simple)
		stream.read()
		if i%3 == 0 {
			// Close the newest stream too, so the player sometimes has no stream at all.
			stream.cancel()
			<-stream.done
		}
	}
	close(stop)
	spammers.Wait()

	unexpected.Range(func(key, _ any) bool {
		t.Errorf("unexpected subscription RPC error: %v", key)
		return true
	})
	assert.Positive(t, accepted.Load(), "no subscription RPC reached an open stream in %d calls", calls.Load())

	final := h.open(t, "player-1", simple)
	final.read()
	h.publish(t, testutils.SimpleEvent{Value: 3}, "player-1")
	assert.Equal(t, []string{"simple_event:3"}, final.next(t, 1))
}

// A tick can still hold a subscriber after its handler returns. Sending to that stream must fail
// instead of writing to a finished handler, which panics the tick's goroutine.
func TestEventStream_TickSendsWhileStreamsClose(t *testing.T) {
	t.Parallel()
	h := newStreamHarness(t)

	stop := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			select {
			case <-stop:
				return
			default:
			}
			h.fixture.tr.Publish(context.Background(), testutils.SimpleEvent{Value: 1}, "player-1")
		}
	}()

	for range 300 {
		// The handler writes the initial empty message before any event queued for it.
		stream := h.open(t, "player-1", "*")
		stream.read()
		stream.cancel()
		<-stream.done
	}
	close(stop)
	<-stopped

	final := h.open(t, "player-1", "*")
	final.read()
	h.publish(t, testutils.SimpleEvent{Value: 5}, "player-1")
	assert.Equal(t, []string{"simple_event:5"}, final.next(t, 1))
}

// A stream whose client stopped reading pins its handler in a blocked write once flow control
// fills. Replacing it must release that handler within the 1 s teardown target, and the tick's
// sends to it never block meanwhile. Not parallel: it asserts timing bounds.
func TestEventStream_StoppedReaderTornDownAfterReplacement(t *testing.T) {
	h := newStreamHarness(t)

	stalled := h.open(t, "player-1", "*") // Never read again.
	subscriber := h.subscriber(t, "player-1")
	payload := testutils.AnotherEvent{Data: incompressible(64 << 10)}
	sends := make([]time.Duration, 0, 200)
	for range 200 { // 12.5 MiB: more than the transport buffers, less than the queue bound.
		start := time.Now()
		h.publish(t, payload, "player-1")
		sends = append(sends, time.Since(start))
	}
	// A blocked send lasts until the stream is replaced (seconds, before the writer queue). The
	// slowest one is bounded loosely: under -race on a loaded host, the scheduler and GC alone
	// can delay a send by a few hundred milliseconds.
	assert.Less(t, percentile(sends, 99), 50*time.Millisecond, "sends to a stopped reader blocked the tick")
	assert.Less(t, percentile(sends, 100), time.Second, "a send to a stopped reader blocked the tick")
	require.Eventually(t, func() bool { return queuedBytes(subscriber) > 0 }, 5*time.Second, time.Millisecond,
		"the stopped reader's writer never fell behind")
	time.Sleep(300 * time.Millisecond)
	require.Positive(t, queuedBytes(subscriber), "the stopped reader's writer is not blocked")

	replacement := h.open(t, "player-1", "*")
	replacedAt := time.Now()
	replacement.read()

	exitedAt := h.awaitHandlerExit(t, stalled.id, 5*time.Second)
	t.Logf("old handler exited %v after replacement", exitedAt.Sub(replacedAt))
	assert.Less(t, exitedAt.Sub(replacedAt), streamTeardownGrace+500*time.Millisecond) // Slack for -race.

	h.publish(t, testutils.SimpleEvent{Value: 4}, "player-1")
	assert.Equal(t, []string{"simple_event:4"}, replacement.next(t, 1))
	replacement.requireOpen(t)
}

// A stopped reader's unwritten events are bounded. Once they pass streamQueueBytes its stream ends
// as lagged at once, well before streamLagLimit, without blocking the tick. The other player's
// stream keeps receiving. Not parallel: it asserts timing bounds.
func TestEventStream_QueueOverflowEndsStoppedStreamAsLagged(t *testing.T) {
	h := newStreamHarness(t)

	stalled := h.open(t, "player-1", "*") // Never read again.
	subscriber := h.subscriber(t, "player-1")
	other := h.open(t, "player-2", "*")
	other.read()

	const eventBytes = 64 << 10
	payload := testutils.AnotherEvent{Data: incompressible(eventBytes)}
	start := time.Now()
	for range 200 { // 12.5 MiB: more than the transport buffers, less than the queue bound.
		h.publish(t, payload, "player-1")
	}
	require.Eventually(t, func() bool { return queuedBytes(subscriber) > 0 }, 5*time.Second, time.Millisecond,
		"the stopped reader's writer never fell behind")
	time.Sleep(300 * time.Millisecond)
	require.NoError(t, subscriber.ctx.Err(), "the stream ended before its queue bound")
	require.Positive(t, queuedBytes(subscriber), "the stopped reader's writer is not blocked")

	published := 0
	for ; subscriber.ctx.Err() == nil && published < 1024; published++ { // 64 MiB at most.
		h.publish(t, payload, "player-1")
	}
	took := time.Since(start)
	t.Logf("stream ended as lagged after %d more events, %v after the first", published, took)

	require.ErrorIs(t, context.Cause(subscriber.ctx), errStreamLagged)
	assert.Less(t, took, streamLagLimit/2, "the stream ended by age, not by its queue bound")
	h.awaitHandlerExit(t, stalled.id, streamTeardownGrace+time.Second)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(h.trySubscribe("player-1", "*")))

	// A reader that resumes drains what its transport buffered, then sees the stream's end.
	for stalled.stream.Receive() {
	}
	requireStoppedReaderEnd(t, stalled.stream.Err())

	h.publish(t, testutils.SimpleEvent{Value: 8}, "")
	assert.Equal(t, []string{"simple_event:8"}, other.next(t, 1))
	other.requireOpen(t)
}

// One tick can queue thousands of events for one stream, as a resync of a dense wave does, far
// faster than the stream's writer sends them. A client that keeps reading receives all of them in
// order, and its stream stays open. Not parallel: the writer must send the burst within
// streamLagLimit, and the race detector makes each write 1-2 ms.
func TestEventStream_LargeTickBurstReachesReadingClient(t *testing.T) {
	h := newStreamHarness(t)

	stream := h.open(t, "player-1", "*")
	stream.read()

	// More than the 1024-event bound this queue used to have. Under -race a loaded host writes
	// about 2 ms per event, so a much larger burst would end by streamLagLimit instead.
	const burst = 1280
	want := make([]string, burst)
	for i := range burst {
		h.fixture.tr.Publish(context.Background(), testutils.SimpleEvent{Value: i}, "player-1")
		want[i] = "simple_event:" + strconv.Itoa(i)
	}

	require.Equal(t, want, stream.next(t, burst))
	stream.requireOpen(t)
}

// Each stream receives its events in publish order, whether broadcast or sent to it directly.
func TestEventStream_OrderPreservedUnderLoad(t *testing.T) {
	t.Parallel()
	h := newStreamHarness(t)

	both := h.open(t, "player-1", "*")
	both.read()
	broadcasts := h.open(t, "player-2", "*")
	broadcasts.read()

	// The server buffers each burst while the test reads the previous one.
	const bursts, perBurst = 10, 512
	for b := range bursts {
		var wantBoth, wantBroadcasts []string
		for i := range perBurst {
			value := b*perBurst + i
			recipient := "" // Even values are broadcast, odd values go to player-1 only.
			if value%2 == 1 {
				recipient = "player-1"
			}
			h.publish(t, testutils.SimpleEvent{Value: value}, recipient)
			wantBoth = append(wantBoth, "simple_event:"+strconv.Itoa(value))
			if recipient == "" {
				wantBroadcasts = append(wantBroadcasts, "simple_event:"+strconv.Itoa(value))
			}
		}
		require.Equal(t, wantBoth, both.next(t, perBurst))
		require.Equal(t, wantBroadcasts, broadcasts.next(t, perBurst/2))
	}
	both.requireOpen(t)
	broadcasts.requireOpen(t)
}

// One stopped reader must not slow the world tick or another player's events. Once its oldest
// pending event has waited streamLagLimit, its stream ends as lagged and frees its handler.
// Not parallel: it asserts timing bounds.
func TestEventStream_StoppedReaderDoesNotStallTick(t *testing.T) {
	h := newStreamHarness(t)

	healthy := recordArrivals(h.open(t, "healthy", "*"))
	stopper := h.open(t, "stopper", "*")
	stopperReader := recordArrivals(stopper)
	// 2.5 MiB/s per subscriber fills the stopped reader's transport buffers within about 2 s.
	ticks := startTicks(t, h, testutils.AnotherEvent{Data: incompressible(128 << 10)})

	time.Sleep(500 * time.Millisecond)
	stopperReader.stop()
	stoppedAt := time.Now()

	exitedAt := h.awaitHandlerExit(t, stopper.id, 2*streamLagLimit)
	time.Sleep(time.Second) // Keep ticking after the stopped stream is gone.
	samples := ticks.stop()
	arrivals := healthy.times()

	t.Logf("stopped stream ended %v after its reader stopped", exitedAt.Sub(stoppedAt))
	assert.Greater(t, exitedAt.Sub(stoppedAt), streamLagLimit-testTickRate)
	assert.Less(t, exitedAt.Sub(stoppedAt), 2*streamLagLimit)
	require.Greater(t, len(samples), 100)
	// Before the writer queue, the stopped reader stalled dispatch and the healthy stream for
	// seconds (4.2 s measured). The slowest tick is bounded loosely: under -race on a loaded host,
	// the scheduler and GC alone can delay one by a few hundred milliseconds.
	took := tookBetween(samples, time.Time{}, time.Now())
	assert.Less(t, percentile(took, 99), testTickRate, "dispatch p99")
	assert.Less(t, percentile(took, 100), time.Second, "slowest dispatch")
	assert.Less(t, maxGap(arrivals, stoppedAt, time.Now()), time.Second, "healthy stream's longest gap")
	assert.Greater(t, len(arrivals), len(samples)-5)

	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(h.trySubscribe("stopper", "*")))
	healthy.requireReading(t)

	// A reader that resumes drains what its transport buffered, then sees the stream's end.
	<-stopperReader.done
	for stopper.stream.Receive() {
	}
	requireStoppedReaderEnd(t, stopper.stream.Err())
}

// A client that keeps reading but is sent more than streamQueueBytes in one burst sees its stream
// end with the retryable "lagged" status, not a clean close.
func TestEventStream_OverflowEndsReadingStreamAsLagged(t *testing.T) {
	t.Parallel()
	h := newStreamHarness(t)

	stream := h.open(t, "player-1", "*")
	subscriber := h.subscriber(t, "player-1")
	reader := recordArrivals(stream)

	payload := testutils.AnotherEvent{Data: incompressible(64 << 10)}
	for published := 0; subscriber.ctx.Err() == nil && published < 1024; published++ { // 64 MiB at most.
		h.publish(t, payload, "player-1")
	}
	require.ErrorIs(t, context.Cause(subscriber.ctx), errStreamLagged)

	select {
	case <-reader.done:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "lagged stream did not end")
	}
	err := stream.stream.Err()
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	assert.ErrorContains(t, err, "lagged")
}

// -------------------------------------------------------------------------------------------------
// Stopped-reader measurement
// -------------------------------------------------------------------------------------------------
// Records how one stopped reader affects the world tick and another player's event cadence, and
// how long its stream lives. Run with CARDINAL_STREAM_STALL_MEASURE=1 go test -run
// StoppedReaderStallMeasurement -v.
// -------------------------------------------------------------------------------------------------

func TestEventStream_StoppedReaderStallMeasurement(t *testing.T) {
	if os.Getenv("CARDINAL_STREAM_STALL_MEASURE") == "" {
		t.Skip("set CARDINAL_STREAM_STALL_MEASURE=1 to measure stopped-reader tick stalls")
	}
	h := newStreamHarness(t)

	const (
		payloadBytes = 16 << 10        // Per broadcast event.
		readingFor   = 2 * time.Second // Both clients read.
		afterFor     = 2 * time.Second // After replacement.
	)

	healthy := recordArrivals(h.open(t, "healthy", "*"))
	stopper := h.open(t, "stopper", "*")
	stopperReader := recordArrivals(stopper)
	ticks := startTicks(t, h, testutils.AnotherEvent{Data: incompressible(payloadBytes)})

	time.Sleep(readingFor)
	stopperReader.stop()
	stoppedAt := time.Now()

	exitedAt := h.awaitHandlerExit(t, stopper.id, time.Minute)
	replacement := recordArrivals(h.open(t, "stopper", "*"))
	replacedAt := time.Now()
	time.Sleep(afterFor)
	samples := ticks.stop()
	arrivals := healthy.times()

	before := tookBetween(samples, time.Time{}, stoppedAt)
	stopped := tookBetween(samples, stoppedAt, replacedAt)
	after := tookBetween(samples, replacedAt, time.Now())
	t.Logf("tick rate %v, %d KiB broadcast per tick, 2 subscribers", testTickRate, payloadBytes>>10)
	t.Logf("both reading: dispatch p50 %v, max %v; healthy max inter-arrival %v",
		percentile(before, 50), percentile(before, 100), maxGap(arrivals, time.Time{}, stoppedAt))
	t.Logf("stopper stopped reading: its stream ended %v later; dispatch p50 %v, max %v over %d ticks",
		exitedAt.Sub(stoppedAt), percentile(stopped, 50), percentile(stopped, 100), len(stopped))
	t.Logf("healthy subscriber max inter-arrival while stopped: %v", maxGap(arrivals, stoppedAt, replacedAt))
	t.Logf("after replacement: dispatch p50 %v, max %v over %d ticks; replacement received %d events",
		percentile(after, 50), percentile(after, 100), len(after), len(replacement.times()))
}

// -------------------------------------------------------------------------------------------------
// Harness
// -------------------------------------------------------------------------------------------------

type streamHarness struct {
	fixture *transportFixture
	client  cardinalv1connect.CardinalServiceClient
	ctx     context.Context
	cancel  context.CancelFunc
	nextID  atomic.Int64

	exitsMu sync.Mutex
	exits   map[string]time.Time // Test stream ID -> when its ServeHTTP returned.
}

func newStreamHarness(t *testing.T) *streamHarness {
	t.Helper()
	fixture := newTransportFixture(t, testutils.NewRand(t), false)
	path, handler, err := fixture.tr.Handler()
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	h := &streamHarness{fixture: fixture, exits: make(map[string]time.Time)}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r)
		if id := r.Header.Get(testStreamIDHeader); id != "" {
			h.exitsMu.Lock()
			h.exits[id] = time.Now()
			h.exitsMu.Unlock()
		}
	}))
	server.Config.Protocols = h2cProtocols()
	server.Start()
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
	})

	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	transport := &http.Transport{Protocols: protocols}
	t.Cleanup(transport.CloseIdleConnections)
	h.client = cardinalv1connect.NewCardinalServiceClient(
		&http.Client{Transport: transport}, server.URL, connect.WithGRPC())

	h.ctx, h.cancel = context.WithCancel(context.Background())
	t.Cleanup(h.cancel) // Runs first: ends every client stream before the server closes.
	return h
}

func (h *streamHarness) subscriptions(events ...string) []*cardinalv1.EventSubscription {
	return []*cardinalv1.EventSubscription{{Address: h.fixture.address, Events: events}}
}

// open starts a stream and waits for its initial empty message, which the server sends only after
// registering the stream.
func (h *streamHarness) open(t *testing.T, player string, events ...string) *clientStream {
	t.Helper()
	stream, err := h.tryOpen(player, events...)
	require.NoError(t, err)
	return stream
}

func (h *streamHarness) tryOpen(player string, events ...string) (*clientStream, error) {
	ctx, cancel := context.WithCancel(h.ctx)
	req := connect.NewRequest(&cardinalv1.StartEventStreamRequest{Subscriptions: h.subscriptions(events...)})
	req.Header().Set(DevPlayerIDHeader, player)
	id := strconv.FormatInt(h.nextID.Add(1), 10)
	req.Header().Set(testStreamIDHeader, id)

	stream, err := h.client.StartEventStream(ctx, req)
	if err != nil {
		cancel()
		return nil, err
	}
	if !stream.Receive() {
		cancel()
		return nil, errors.Join(errors.New("stream closed before its initial message"), stream.Err())
	}
	if stream.Msg().GetEvent() != nil {
		cancel()
		return nil, errors.New("initial message must be empty")
	}
	return &clientStream{
		id:       id,
		ctx:      ctx,
		stream:   stream,
		cancel:   cancel,
		messages: make(chan *cardinalv1.StartEventStreamResponse, 64),
		done:     make(chan struct{}),
	}, nil
}

func (h *streamHarness) publish(t *testing.T, payload Payload, recipient string) {
	t.Helper()
	h.fixture.tr.Publish(context.Background(), payload, recipient)
}

func (h *streamHarness) trySubscribe(player string, events ...string) error {
	req := connect.NewRequest(&cardinalv1.SubscribeEventsRequest{Subscriptions: h.subscriptions(events...)})
	req.Header().Set(DevPlayerIDHeader, player)
	_, err := h.client.SubscribeEvents(h.ctx, req)
	return err
}

func (h *streamHarness) tryUnsubscribe(player string, events ...string) error {
	req := connect.NewRequest(&cardinalv1.UnsubscribeEventsRequest{Subscriptions: h.subscriptions(events...)})
	req.Header().Set(DevPlayerIDHeader, player)
	_, err := h.client.UnsubscribeEvents(h.ctx, req)
	return err
}

func (h *streamHarness) subscribe(t *testing.T, player string, events ...string) {
	t.Helper()
	require.NoError(t, h.trySubscribe(player, events...))
}

// awaitHandlerExit returns when the server's handler for stream id returned.
func (h *streamHarness) awaitHandlerExit(t *testing.T, id string, within time.Duration) time.Time {
	t.Helper()
	var exited time.Time
	require.Eventually(t, func() bool {
		h.exitsMu.Lock()
		defer h.exitsMu.Unlock()
		var ok bool
		exited, ok = h.exits[id]
		return ok
	}, within, time.Millisecond, "handler for stream %s never returned", id)
	return exited
}

type clientStream struct {
	id       string
	ctx      context.Context
	stream   *connect.ServerStreamForClient[cardinalv1.StartEventStreamResponse]
	cancel   context.CancelFunc
	messages chan *cardinalv1.StartEventStreamResponse
	done     chan struct{}
	err      error // Set before done closes.
}

// read starts delivering the stream's messages to messages, then its final error.
func (c *clientStream) read() {
	go func() {
		defer close(c.done)
		for c.stream.Receive() {
			select {
			case c.messages <- c.stream.Msg():
			case <-c.ctx.Done():
			}
		}
		c.err = c.stream.Err()
	}()
}

// next returns the next n events, described as "name:value".
func (c *clientStream) next(t *testing.T, n int) []string {
	t.Helper()
	var got []string
	for len(got) < n {
		select {
		case msg := <-c.messages:
			if msg.GetEvent() != nil { // Skip keepalives.
				got = append(got, describe(msg))
			}
		case <-c.done:
			require.FailNow(t, "stream ended early", "after %v: %v", got, c.err)
		case <-time.After(5 * time.Second):
			require.FailNow(t, "timed out waiting for events", "got %v", got)
		}
	}
	return got
}

func (c *clientStream) requireOpen(t *testing.T) {
	t.Helper()
	select {
	case <-c.done:
		require.FailNow(t, "stream ended", "%v", c.err)
	default:
	}
}

func requireSuperseded(t *testing.T, c *clientStream) {
	t.Helper()
	select {
	case <-c.done:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "superseded stream did not end")
	}
	require.Error(t, c.err)
	assert.Equal(t, connect.CodeAborted, connect.CodeOf(c.err))
	assert.Contains(t, c.err.Error(), "superseded")
}

// describe renders an event as "name:value" with its decoded payload.
func describe(msg *cardinalv1.StartEventStreamResponse) string {
	evt := msg.GetEvent()
	switch evt.GetName() {
	case testutils.SimpleEvent{}.Name():
		value, err := testutils.SimpleEvent{}.UnmarshalWire(evt.GetPayload())
		if err != nil {
			return evt.GetName() + ":" + err.Error()
		}
		return evt.GetName() + ":" + strconv.Itoa(value.(testutils.SimpleEvent).Value)
	case testutils.AnotherEvent{}.Name():
		value, _ := testutils.AnotherEvent{}.UnmarshalWire(evt.GetPayload())
		if data := value.(testutils.AnotherEvent).Data; len(data) <= 32 {
			return evt.GetName() + ":" + data
		}
		return evt.GetName() + ":<" + strconv.Itoa(len(evt.GetPayload())) + " bytes>"
	default:
		return evt.GetName() + ":?"
	}
}

// requireStoppedReaderEnd checks how a stopped reader's stream ended once it reads again. If the
// handler was blocked in a write, the "lagged" status cannot pass the full flow-control window,
// so the forced teardown resets the stream. If the writer was still sending when the stream
// ended, the status fits in what is left of the window.
func requireStoppedReaderEnd(t *testing.T, err error) {
	t.Helper()
	if connect.CodeOf(err) == connect.CodeUnavailable {
		require.ErrorContains(t, err, "lagged")
		return
	}
	require.Equal(t, connect.CodeInternal, connect.CodeOf(err), "%v", err)
	require.ErrorContains(t, err, "INTERNAL_ERROR")
}

// queuedBytes returns the bytes charged for a stream's unwritten events.
func queuedBytes(subscriber *streamSubscriber) int {
	subscriber.queueMu.Lock()
	defer subscriber.queueMu.Unlock()
	return subscriber.queuedBytes
}

// subscriber returns player's registered stream.
func (h *streamHarness) subscriber(t *testing.T, player string) *streamSubscriber {
	t.Helper()
	h.fixture.tr.clients.mu.RLock()
	defer h.fixture.tr.clients.mu.RUnlock()
	subscriber, ok := h.fixture.tr.clients.subscribers[player]
	require.True(t, ok, "%s has no stream", player)
	return subscriber
}

const testTickRate = 50 * time.Millisecond // 20 Hz.

type tickSample struct {
	start time.Time
	took  time.Duration
}

// tickRecorder broadcasts one event per tick through Publish, like a running world's event
// dispatch, and records how long each dispatch took.
type tickRecorder struct {
	mu      sync.Mutex
	samples []tickSample
	quit    chan struct{}
	stopped chan struct{}
}

func startTicks(t *testing.T, h *streamHarness, payload Payload) *tickRecorder {
	t.Helper()
	r := &tickRecorder{quit: make(chan struct{}), stopped: make(chan struct{})}
	t.Cleanup(func() {
		select {
		case <-r.quit:
		default:
			r.stop()
		}
	})
	go func() {
		defer close(r.stopped)
		next := time.Now()
		for {
			select {
			case <-r.quit:
				return
			case <-time.After(time.Until(next)):
			}
			next = next.Add(testTickRate)
			start := time.Now()
			h.fixture.tr.Publish(context.Background(), payload, "")
			r.mu.Lock()
			r.samples = append(r.samples, tickSample{start: start, took: time.Since(start)})
			r.mu.Unlock()
		}
	}()
	return r
}

// stop ends the ticks and returns the samples.
func (r *tickRecorder) stop() []tickSample {
	close(r.quit)
	<-r.stopped
	return r.samples
}

func tookBetween(samples []tickSample, from, to time.Time) []time.Duration {
	var took []time.Duration
	for _, s := range samples {
		if s.start.After(from) && s.start.Before(to) {
			took = append(took, s.took)
		}
	}
	return took
}

// arrivalRecorder reads a stream until stopped and records when each event arrived.
type arrivalRecorder struct {
	reading  atomic.Bool
	mu       sync.Mutex
	arrivals []time.Time
	done     chan struct{}
}

func recordArrivals(c *clientStream) *arrivalRecorder {
	r := &arrivalRecorder{done: make(chan struct{})}
	r.reading.Store(true)
	go func() {
		defer close(r.done)
		for r.reading.Load() && c.stream.Receive() {
			if c.stream.Msg().GetEvent() != nil {
				r.mu.Lock()
				r.arrivals = append(r.arrivals, time.Now())
				r.mu.Unlock()
			}
		}
	}()
	return r
}

// stop stops reading: the client takes no more events off the stream.
func (r *arrivalRecorder) stop() { r.reading.Store(false) }

func (r *arrivalRecorder) times() []time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.arrivals)
}

func (r *arrivalRecorder) requireReading(t *testing.T) {
	t.Helper()
	select {
	case <-r.done:
		require.FailNow(t, "stream ended")
	default:
	}
}

// incompressible returns n random bytes. The Connect client negotiates gzip, which would shrink a
// repetitive payload far below the stream's flow-control window.
func incompressible(n int) string {
	b := make([]byte, n)
	_, _ = cryptorand.Read(b)
	return string(b)
}

func maxGap(times []time.Time, from, to time.Time) time.Duration {
	var gap time.Duration
	prev := from
	for _, at := range times {
		if at.Before(from) {
			continue
		}
		if at.After(to) {
			break
		}
		if !prev.IsZero() && at.Sub(prev) > gap {
			gap = at.Sub(prev)
		}
		prev = at
	}
	return gap
}

func percentile(durations []time.Duration, p int) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	sorted := slices.Clone(durations)
	slices.Sort(sorted)
	return sorted[(len(sorted)-1)*p/100]
}

// h2cProtocols enables HTTP/1.1 and unencrypted HTTP/2 (h2c), as Cardinal's server does.
func h2cProtocols() *http.Protocols {
	p := new(http.Protocols)
	p.SetHTTP1(true)
	p.SetUnencryptedHTTP2(true)
	return p
}
