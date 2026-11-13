package cardinal

import (
	"context"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Dead connections
// -------------------------------------------------------------------------------------------------
// A client can vanish without closing its connection: a phone leaves coverage, a frozen app keeps
// its socket. These tests break the server's side of one connection the way such a network does,
// with no FIN and no reset, and check what the server still holds for that client.
// -------------------------------------------------------------------------------------------------

// A client whose connection goes silent while the shard is writing to it (a phone out of coverage)
// pins its stream's handler in a write. Cancelling the stream cannot end that write: its reset must
// be written to the same stuck connection. The server closes a connection whose writes make no
// progress, so the handler returns, the stream leaves the table, the player's sync reader leaves
// its seat and nothing holds the stream's queue. Not parallel: it asserts timing bounds.
func TestEventStream_StalledConnectionReleasesItsStream(t *testing.T) {
	w := NewTestWorld(t, func(w *World) {
		w.RegisterComponent[syncHealth]()
		w.RegisterComponent[syncPrivate]()
		w.EnableSync(func(string) bool { return true })
	})
	// The pings keep their production timing, far longer than this test, so only the write
	// timeout can close the connection.
	const writeTimeout = time.Second
	h := startStreamHarness(t, &serviceFixture{svc: w.service, world: w.World},
		shortenConnTimeouts(writeTimeout, 0, 0))
	avatar := w.Create[syncAvatar]()

	stream, conn := h.openOnOwnConn(t, "alice", syncstate.EventName)
	stream.read()
	w.Tick()
	require.NotNil(t, receiveSyncFrame(t, stream).Open)
	subscriber := h.subscriber(t, "alice")

	conn.stall()
	stalledAt := time.Now()
	// Every tick changes the avatar, so every tick queues a delta that cannot be written.
	for hp := 1; h.handlerRunning(stream.id) && time.Since(stalledAt) < writeTimeout+streamLagLimit; hp++ {
		avatar.Set(syncHealth{HP: hp})
		w.Tick()
		time.Sleep(testTickRate)
	}
	exitedAt := h.awaitHandlerExit(t, stream.id, testTickRate)
	t.Logf("handler returned %v after the connection stalled", exitedAt.Sub(stalledAt))
	assert.Less(t, exitedAt.Sub(stalledAt), writeTimeout+time.Second) // Slack for -race.

	w.Tick()
	assert.False(t, seated(w.World, "alice"), "the player's sync reader kept its seat")
	h.fixture.svc.mu.RLock()
	_, registered := h.fixture.svc.subscribers["alice"]
	h.fixture.svc.mu.RUnlock()
	assert.False(t, registered, "the stream stayed in the table")
	assert.Zero(t, queuedBytes(subscriber), "the stream kept its queue")
}

// A client that stops answering while nothing is being written to it, so no write can stall, is
// found by the server's ping: its connection closes and its stream unwinds. Until then the server's
// pings reach a live idle client, which answers them and keeps its stream. Not parallel: it asserts
// timing bounds.
func TestEventStream_SilentIdleConnectionClosedByPing(t *testing.T) {
	const pingAfter, pingTimeout = 500 * time.Millisecond, 2 * time.Second
	h := newStreamHarness(t, shortenConnTimeouts(0, pingAfter, pingTimeout))
	stream, conn := h.openOnOwnConn(t, "player-1", "*")
	stream.read()

	written := conn.written()
	time.Sleep(2 * (pingAfter + pingTimeout))
	stream.requireOpen(t)
	require.Greater(t, conn.written(), written, "the server never pinged the idle connection")

	conn.silence()
	silencedAt := time.Now()
	exitedAt := h.awaitHandlerExit(t, stream.id, 2*(pingAfter+pingTimeout))
	t.Logf("handler returned %v after the connection fell silent", exitedAt.Sub(silencedAt))
	assert.Less(t, exitedAt.Sub(silencedAt), pingAfter+pingTimeout+time.Second) // Slack for -race.
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(h.trySubscribe("player-1", "*")),
		"the stream stayed in the table")
}

// A client on a slow link reads every event, late. Neither connection timeout closes it, though a
// single frame takes longer than the write timeout to cross the link. Not parallel: it asserts
// timing bounds.
func TestEventStream_SlowReaderKeepsItsConnection(t *testing.T) {
	const writeTimeout, pingAfter, pingTimeout = 500 * time.Millisecond, 500 * time.Millisecond, 2 * time.Second
	h := newStreamHarness(t, shortenConnTimeouts(writeTimeout, pingAfter, pingTimeout))
	stream, conn := h.openOnOwnConn(t, "player-1", "*")
	stream.read()

	// 20 KiB/s: a full 16 KiB frame takes 800 ms, and every 50 ms some bytes get through.
	conn.slow(1<<10, 50*time.Millisecond)
	start := time.Now()
	const events = 6 // 60 KiB: about three seconds on this link.
	for range events {
		h.publish(t, testutils.AnotherEvent{Data: incompressible(10 << 10)}, "player-1")
	}
	for _, got := range stream.next(t, events) {
		assert.Contains(t, got, "another_event:<")
	}
	took := time.Since(start)
	t.Logf("%d events took %v", events, took)
	require.Greater(t, took, pingAfter+pingTimeout, "the link was not slow enough to test the timeouts")
	stream.requireOpen(t)
}

// A stream cut as lagged while its handler is blocked writing to a dead connection gives up its
// sync reader seat at the next tick, before anything closes the connection.
func TestSync_LaggedStreamLeavesSeatWhileHandlerBlocked(t *testing.T) {
	t.Parallel()
	w := NewTestWorld(t, func(w *World) {
		w.RegisterComponent[syncHealth]()
		w.RegisterComponent[syncPrivate]()
		w.EnableSync(func(string) bool { return true })
	})
	h := startStreamHarness(t, &serviceFixture{svc: w.service, world: w.World})
	another := testutils.AnotherEvent{}.Name()
	w.Create[syncAvatar]()

	stream, conn := h.openOnOwnConn(t, "alice", syncstate.EventName, another)
	stream.read()
	w.Tick()
	require.NotNil(t, receiveSyncFrame(t, stream).Open)
	require.True(t, seated(w.World, "alice"))
	subscriber := h.subscriber(t, "alice")

	conn.stall()
	cutLagged(t, h, "alice", subscriber)
	w.Tick()

	assert.False(t, seated(w.World, "alice"), "a cancelled stream kept its sync reader seat")
	assert.True(t, h.handlerRunning(stream.id), "the handler returned, so this did not test a blocked one")
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(h.trySubscribe("alice", another)),
		"a cancelled stream accepted a subscription")
}

// A cancelled stream drops its unwritten events at once, so a handler blocked writing to a dead
// connection holds the one event it is writing, not the stream's backlog.
func TestEventStream_CancelledStreamDropsQueue(t *testing.T) {
	t.Parallel()
	h := newStreamHarness(t)
	stream, conn := h.openOnOwnConn(t, "player-1", "*")
	subscriber := h.subscriber(t, "player-1")

	conn.stall()
	cutLagged(t, h, "player-1", subscriber)

	require.Eventually(t, func() bool { return queuedBytes(subscriber) == 0 }, time.Second, time.Millisecond,
		"the cancelled stream kept its queue")
	h.publish(t, testutils.AnotherEvent{Data: incompressible(64 << 10)}, "player-1")
	assert.Zero(t, queuedBytes(subscriber), "the cancelled stream queued an event")
	assert.True(t, h.handlerRunning(stream.id), "the handler returned, so this did not test a blocked one")
}

// cutLagged fills subscriber's queue until its stream ends as lagged. Its connection must be
// stalled, so nothing is written meanwhile.
func cutLagged(t *testing.T, h *streamHarness, player string, subscriber *streamSubscriber) {
	t.Helper()
	payload := testutils.AnotherEvent{Data: incompressible(64 << 10)}
	for published := 0; subscriber.ctx.Err() == nil && published < 512; published++ { // 32 MiB at most.
		h.publish(t, payload, player)
	}
	require.ErrorIs(t, context.Cause(subscriber.ctx), errStreamLagged)
}

// seated reports whether player's sync reader holds a seat. Call it between ticks.
func seated(w *World, player string) bool {
	slot, ok := w.sync.roster.slotOf[player]
	return ok && w.sync.roster.live.has(slot)
}

// -------------------------------------------------------------------------------------------------
// Breakable connections
// -------------------------------------------------------------------------------------------------

// pathListener hands the server connections a test can break, and finds them by client address.
type pathListener struct {
	net.Listener

	mu    sync.Mutex
	conns map[string]*pathConn // By remote address.
}

func (l *pathListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	c := &pathConn{Conn: conn, wake: make(chan struct{}), closed: make(chan struct{})}
	l.mu.Lock()
	l.conns[conn.RemoteAddr().String()] = c
	l.mu.Unlock()
	return c, nil
}

// conn returns the server's side of the connection the client dialed from clientAddr.
func (l *pathListener) conn(t *testing.T, clientAddr string) *pathConn {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	c, ok := l.conns[clientAddr]
	require.True(t, ok, "the server never accepted a connection from %s", clientAddr)
	return c
}

type pathState int

const (
	pathOpen    pathState = iota // Bytes pass both ways.
	pathSlow                     // Bytes pass both ways, but writes pass slowly.
	pathSilent                   // Nothing arrives, and writes vanish: the path still has room.
	pathStalled                  // Nothing arrives, and writes make no progress: the path is full.
)

// pathConn is the server's side of one connection, carried by a network a test controls. A broken
// path never recovers, so bytes the client sent after the break are never delivered.
type pathConn struct {
	net.Conn

	mu            sync.Mutex
	state         pathState     // Guarded by mu.
	chunk         int           // Guarded by mu. On a slow path, bytes written per gap.
	gap           time.Duration // Guarded by mu.
	writeDeadline time.Time     // Guarded by mu.
	wrote         int           // Guarded by mu. Bytes the server wrote, delivered or not.
	wake          chan struct{} // Guarded by mu. Closed and replaced on every change.
	closeOnce     sync.Once
	closed        chan struct{}
}

// stall fills the path: the client receives nothing more and sends nothing that arrives.
func (c *pathConn) stall() { c.change(func() { c.state = pathStalled }) }

// silence cuts the path while it still has room: the server's writes succeed, but nothing reaches
// the client, and the client sends nothing that arrives.
func (c *pathConn) silence() { c.change(func() { c.state = pathSilent }) }

// slow lets the server write chunk bytes per gap.
func (c *pathConn) slow(chunk int, gap time.Duration) {
	c.change(func() { c.state, c.chunk, c.gap = pathSlow, chunk, gap })
}

// written returns how many bytes the server has written to the connection.
func (c *pathConn) written() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.wrote
}

func (c *pathConn) change(f func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	f()
	close(c.wake)
	c.wake = make(chan struct{})
}

func (c *pathConn) now() (pathState, time.Time, chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state, c.writeDeadline, c.wake
}

func (c *pathConn) Read(b []byte) (int, error) {
	for {
		state, _, wake := c.now()
		if state == pathOpen || state == pathSlow {
			n, err := c.Conn.Read(b)
			if after, _, _ := c.now(); (after == pathSilent || after == pathStalled) && err == nil {
				continue // It arrived after the break.
			}
			return n, err
		}
		select {
		case <-wake:
		case <-c.closed:
			return 0, net.ErrClosed
		}
	}
}

func (c *pathConn) Write(b []byte) (int, error) {
	n, err := c.write(b)
	c.mu.Lock()
	c.wrote += n
	c.mu.Unlock()
	return n, err
}

func (c *pathConn) write(b []byte) (int, error) {
	for {
		state, deadline, wake := c.now()
		switch state {
		case pathOpen:
			return c.Conn.Write(b)
		case pathSlow:
			return c.writeSlowly(b, deadline)
		case pathSilent:
			return len(b), nil
		case pathStalled:
		}
		if err := c.waitUntil(deadline, wake, 0); err != nil {
			return 0, err
		}
	}
}

// writeSlowly writes b a chunk per gap. Like a socket on a slow link, a write still going at its
// deadline fails, reporting the bytes it wrote.
func (c *pathConn) writeSlowly(b []byte, deadline time.Time) (int, error) {
	c.mu.Lock()
	chunk, gap := c.chunk, c.gap
	c.mu.Unlock()
	n := 0
	for n < len(b) {
		if err := c.waitUntil(deadline, nil, gap); err != nil {
			return n, err
		}
		m, err := c.Conn.Write(b[n:min(n+chunk, len(b))])
		n += m
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

// waitUntil blocks until the connection changes or, if pause is not zero, pause passes. It fails at
// the write deadline or once the connection is closed.
func (c *pathConn) waitUntil(deadline time.Time, wake chan struct{}, pause time.Duration) error {
	var expired, paused <-chan time.Time
	if !deadline.IsZero() {
		timer := time.NewTimer(time.Until(deadline))
		defer timer.Stop()
		expired = timer.C
	}
	if pause > 0 {
		timer := time.NewTimer(pause)
		defer timer.Stop()
		paused = timer.C
	}
	select {
	case <-wake:
		return nil
	case <-paused:
		return nil
	case <-expired:
		return os.ErrDeadlineExceeded
	case <-c.closed:
		return net.ErrClosed
	}
}

func (c *pathConn) SetWriteDeadline(deadline time.Time) error {
	c.change(func() { c.writeDeadline = deadline })
	return c.Conn.SetWriteDeadline(deadline)
}

func (c *pathConn) SetDeadline(deadline time.Time) error {
	c.change(func() { c.writeDeadline = deadline })
	return c.Conn.SetDeadline(deadline)
}

func (c *pathConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return c.Conn.Close()
}
