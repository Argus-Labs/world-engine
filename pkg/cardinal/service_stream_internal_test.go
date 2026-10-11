package cardinal

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
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
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
			_ = h.fixture.svc.publishDefaultEvent(context.Background(), event.Event{
				Kind: event.KindDefault, Payload: testutils.SimpleEvent{Value: 1}, Recipient: "player-1",
			})
		}
	}()

	for range 300 {
		stream, err := h.tryOpen("player-1", "*")
		if err != nil {
			// An event can reach a registered stream before its initial empty message. That stream
			// was still opened and is now cancelled, which is the cycle this test needs.
			require.EqualError(t, err, "initial message must be empty")
			continue
		}
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

// A stream whose client stopped reading blocks the tick's send once flow control fills. Replacing it
// must release both the old handler and the blocked send within the 1 s teardown target.
func TestEventStream_StoppedReaderTornDownAfterReplacement(t *testing.T) {
	t.Parallel()
	h := newStreamHarness(t)

	stalled := h.open(t, "player-1", "*") // Never read again.
	tick := startPublisher(t, h, "player-1", 64<<10)
	blockedAt := tick.awaitBlocked(t)
	t.Logf("publisher blocked after %d events", tick.published.Load())

	replacement := h.open(t, "player-1", "*")
	replacedAt := time.Now()
	replacement.read()

	exitedAt := h.awaitHandlerExit(t, stalled.id)
	resumedAt := tick.awaitProgressAfter(t, replacedAt)
	t.Logf("old handler exited %v and the publisher resumed %v after replacement (blocked %v before it)",
		exitedAt.Sub(replacedAt), resumedAt.Sub(replacedAt), replacedAt.Sub(blockedAt))
	target := streamTeardownGrace + 500*time.Millisecond // Scheduling slack under -race.
	assert.Less(t, exitedAt.Sub(replacedAt), target)
	assert.Less(t, resumedAt.Sub(replacedAt), target)

	// The tick now delivers to the replacement.
	require.NotEmpty(t, replacement.next(t, 1))
	replacement.requireOpen(t)
}

// -------------------------------------------------------------------------------------------------
// Stopped-reader measurement
// -------------------------------------------------------------------------------------------------
// Records whether one stopped reader stalls the world tick and another player's event cadence.
// Run with CARDINAL_STREAM_STALL_MEASURE=1 go test -run StoppedReaderStallMeasurement -v.
// -------------------------------------------------------------------------------------------------

func TestEventStream_StoppedReaderStallMeasurement(t *testing.T) {
	if os.Getenv("CARDINAL_STREAM_STALL_MEASURE") == "" {
		t.Skip("set CARDINAL_STREAM_STALL_MEASURE=1 to measure stopped-reader tick stalls")
	}
	h := newStreamHarness(t)
	h.fixture.world.events.RegisterHandler(event.KindDefault, h.fixture.svc.publishDefaultEvent)

	const (
		tickRate     = 50 * time.Millisecond // 20 Hz.
		payloadBytes = 16 << 10              // Per broadcast event.
		readingFor   = 2 * time.Second       // Both clients read.
		stalledFor   = 3 * time.Second       // After the first stalled tick, before replacement.
		afterFor     = 2 * time.Second       // After replacement.
	)

	healthy := h.open(t, "healthy", "*")
	var arrivals []time.Time
	var arrivalsMu sync.Mutex
	go func() {
		for healthy.stream.Receive() {
			arrivalsMu.Lock()
			arrivals = append(arrivals, time.Now())
			arrivalsMu.Unlock()
		}
	}()

	stopper := h.open(t, "stopper", "*")
	var stopperReading atomic.Bool
	stopperReading.Store(true)
	go func() {
		for stopperReading.Load() && stopper.stream.Receive() {
		}
	}()

	type tickSample struct {
		start time.Time
		took  time.Duration
	}
	var samples []tickSample
	var samplesMu sync.Mutex
	ticking := make(chan struct{})
	payload := testutils.AnotherEvent{Data: incompressible(payloadBytes)}
	go func() {
		defer close(ticking)
		next := time.Now()
		for {
			select {
			case <-h.ctx.Done():
				return
			default:
			}
			time.Sleep(time.Until(next))
			next = next.Add(tickRate)
			start := time.Now()
			h.fixture.world.events.Enqueue(event.Event{Kind: event.KindDefault, Payload: payload})
			h.fixture.world.dispatchEvents(context.Background())
			samplesMu.Lock()
			samples = append(samples, tickSample{start: start, took: time.Since(start)})
			samplesMu.Unlock()
		}
	}()

	time.Sleep(readingFor)
	stoppedAt := time.Now()
	stopperReading.Store(false)

	// Wait for the first stalled tick: one still running after 5 tick periods.
	var stallStart time.Time
	require.Eventually(t, func() bool {
		samplesMu.Lock()
		defer samplesMu.Unlock()
		last := samples[len(samples)-1]
		if time.Since(last.start.Add(last.took)) > 5*tickRate {
			stallStart = last.start.Add(last.took)
			return true
		}
		return false
	}, 30*time.Second, 10*time.Millisecond, "the tick never stalled")
	time.Sleep(stalledFor)

	replacement := h.open(t, "stopper", "*")
	replacedAt := time.Now()
	go func() {
		for replacement.stream.Receive() {
		}
	}()
	time.Sleep(afterFor)
	h.cancel()
	<-ticking

	samplesMu.Lock()
	arrivalsMu.Lock()
	defer samplesMu.Unlock()
	defer arrivalsMu.Unlock()

	var before, after []time.Duration
	var longest tickSample
	ticksBeforeStall := 0
	for _, s := range samples {
		if s.took > longest.took {
			longest = s
		}
		switch {
		case s.start.Before(stoppedAt):
			before = append(before, s.took)
		case s.start.After(replacedAt):
			after = append(after, s.took)
		}
		if s.start.After(stoppedAt) && s.start.Before(stallStart) {
			ticksBeforeStall++
		}
	}
	healthyGap := maxGap(arrivals, stoppedAt, replacedAt.Add(afterFor))
	baselineGap := maxGap(arrivals, time.Time{}, stoppedAt)

	t.Logf("tick rate %v, %d KiB broadcast per tick, 2 subscribers", tickRate, payloadBytes>>10)
	t.Logf("both reading: dispatch p50 %v, max %v; healthy max inter-arrival %v",
		percentile(before, 50), percentile(before, 100), baselineGap)
	t.Logf("stopper stopped reading: %d ticks (%d KiB) were accepted before dispatch blocked",
		ticksBeforeStall, ticksBeforeStall*payloadBytes>>10)
	t.Logf("longest dispatch %v (started %v after the stop); stalled tick resumed %v after replacement",
		longest.took, longest.start.Sub(stoppedAt), longest.start.Add(longest.took).Sub(replacedAt))
	t.Logf("healthy subscriber max inter-arrival while stalled: %v", healthyGap)
	t.Logf("after replacement: dispatch p50 %v, max %v over %d ticks",
		percentile(after, 50), percentile(after, 100), len(after))
}

// -------------------------------------------------------------------------------------------------
// Harness
// -------------------------------------------------------------------------------------------------

type streamHarness struct {
	fixture *serviceFixture
	client  cardinalv1connect.CardinalServiceClient
	ctx     context.Context
	cancel  context.CancelFunc
	nextID  atomic.Int64

	exitsMu sync.Mutex
	exits   map[string]time.Time // Test stream ID -> when its ServeHTTP returned.
}

func newStreamHarness(t *testing.T) *streamHarness {
	t.Helper()
	fixture := newServiceFixture(t, testutils.NewRand(t), false)
	mux, err := fixture.svc.newMux(authenticatorDev{}.authenticate)
	require.NoError(t, err)

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
	return []*cardinalv1.EventSubscription{{Address: h.fixture.world.address, Events: events}}
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
	req.Header().Set(devPlayerIDHeader, player)
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

func (h *streamHarness) publish(t *testing.T, payload event.Payload, recipient string) {
	t.Helper()
	require.NoError(t, h.fixture.svc.publishDefaultEvent(context.Background(), event.Event{
		Kind: event.KindDefault, Payload: payload, Recipient: recipient,
	}))
}

func (h *streamHarness) trySubscribe(player string, events ...string) error {
	req := connect.NewRequest(&cardinalv1.SubscribeEventsRequest{Subscriptions: h.subscriptions(events...)})
	req.Header().Set(devPlayerIDHeader, player)
	_, err := h.client.SubscribeEvents(h.ctx, req)
	return err
}

func (h *streamHarness) tryUnsubscribe(player string, events ...string) error {
	req := connect.NewRequest(&cardinalv1.UnsubscribeEventsRequest{Subscriptions: h.subscriptions(events...)})
	req.Header().Set(devPlayerIDHeader, player)
	_, err := h.client.UnsubscribeEvents(h.ctx, req)
	return err
}

func (h *streamHarness) subscribe(t *testing.T, player string, events ...string) {
	t.Helper()
	require.NoError(t, h.trySubscribe(player, events...))
}

// awaitHandlerExit returns when the server's handler for stream id returned.
func (h *streamHarness) awaitHandlerExit(t *testing.T, id string) time.Time {
	t.Helper()
	var exited time.Time
	require.Eventually(t, func() bool {
		h.exitsMu.Lock()
		defer h.exitsMu.Unlock()
		var ok bool
		exited, ok = h.exits[id]
		return ok
	}, 5*time.Second, time.Millisecond, "handler for stream %s never returned", id)
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

// publisher sends large SendTo events to one player as fast as possible, like a tick dispatching
// to a stream, and records when each send returned.
type publisher struct {
	published atomic.Int64
	lastDone  atomic.Int64 // UnixNano.
}

func startPublisher(t *testing.T, h *streamHarness, player string, payloadBytes int) *publisher {
	t.Helper()
	p := &publisher{}
	p.lastDone.Store(time.Now().UnixNano())
	payload := testutils.AnotherEvent{Data: incompressible(payloadBytes)}
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for h.ctx.Err() == nil {
			_ = h.fixture.svc.publishDefaultEvent(context.Background(), event.Event{
				Kind: event.KindDefault, Payload: payload, Recipient: player,
			})
			p.published.Add(1)
			p.lastDone.Store(time.Now().UnixNano())
		}
	}()
	t.Cleanup(func() {
		h.cancel() // Ends the client streams, which releases a blocked send.
		<-stopped
	})
	return p
}

// awaitBlocked returns when a send has been in progress for 300 ms.
func (p *publisher) awaitBlocked(t *testing.T) time.Time {
	t.Helper()
	var blockedAt time.Time
	require.Eventually(t, func() bool {
		last := time.Unix(0, p.lastDone.Load())
		blockedAt = last
		return p.published.Load() > 0 && time.Since(last) > 300*time.Millisecond
	}, 20*time.Second, 10*time.Millisecond, "publisher never blocked")
	return blockedAt
}

// awaitProgressAfter returns when a send completed after since.
func (p *publisher) awaitProgressAfter(t *testing.T, since time.Time) time.Time {
	t.Helper()
	var done time.Time
	require.Eventually(t, func() bool {
		done = time.Unix(0, p.lastDone.Load())
		return done.After(since)
	}, 5*time.Second, time.Millisecond, "publisher never resumed")
	return done
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
