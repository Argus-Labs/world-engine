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

// A stream cut as lagged while its handler is blocked writing to a dead connection gives up its
// sync reader seat at the next tick, before anything closes the connection. Not parallel: the
// lag cut needs a burst of 16 MiB.
func TestSync_LaggedStreamLeavesSeatWhileHandlerBlocked(t *testing.T) {
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
	pathStalled                  // Nothing arrives, and writes make no progress: the path is full.
)

// pathConn is the server's side of one connection, carried by a network a test controls. A broken
// path never recovers, so bytes the client sent after the break are never delivered.
type pathConn struct {
	net.Conn

	mu            sync.Mutex
	state         pathState     // Guarded by mu.
	writeDeadline time.Time     // Guarded by mu.
	wake          chan struct{} // Guarded by mu. Closed and replaced on every change.
	closeOnce     sync.Once
	closed        chan struct{}
}

// stall fills the path: the client receives nothing more and sends nothing that arrives.
func (c *pathConn) stall() { c.change(func() { c.state = pathStalled }) }

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
		if state == pathOpen {
			n, err := c.Conn.Read(b)
			if after, _, _ := c.now(); after != pathOpen && err == nil {
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
	for {
		state, deadline, wake := c.now()
		if state == pathOpen {
			return c.Conn.Write(b)
		}
		if err := c.waitUntil(deadline, wake); err != nil {
			return 0, err
		}
	}
}

// waitUntil blocks until the connection changes, or fails at the write deadline or close.
func (c *pathConn) waitUntil(deadline time.Time, wake chan struct{}) error {
	var expired <-chan time.Time
	if !deadline.IsZero() {
		timer := time.NewTimer(time.Until(deadline))
		defer timer.Stop()
		expired = timer.C
	}
	select {
	case <-wake:
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
