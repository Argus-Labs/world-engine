package cardinal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Shutdown with an open event stream
// -------------------------------------------------------------------------------------------------
// The tick loop stops before the service shuts down. A connected player's idle event stream must
// not hold server shutdown for its whole budget, and the NATS endpoints must close first and even
// when the budget runs out, so no other shard gets an ack for a command no tick will process.
// -------------------------------------------------------------------------------------------------

const shutdownTestPlayer = "shutdown-player"

func TestService_ShutdownEndsOpenEventStream(t *testing.T) {
	t.Parallel()

	h := newShutdownHarness(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	err := h.fixture.svc.shutdown(ctx)

	require.NoError(t, err, "an idle event stream must not exhaust the shutdown budget")
	assert.Less(t, time.Since(start), 3*time.Second)
	requireStreamUnavailable(t, h.ended)
	assert.True(t, h.fixture.svc.client.IsClosed(), "shutdown must close the NATS connection")
	requireCommandRejected(t, h.fixture)
}

func TestService_ShutdownClosesNATSAfterDeadline(t *testing.T) {
	t.Parallel()

	h := newShutdownHarness(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Whether server shutdown also fails depends on how far the stream's teardown got.
	_ = h.fixture.svc.shutdown(ctx)

	assert.True(t, h.fixture.svc.client.IsClosed(), "shutdown must close the NATS connection")
	requireCommandRejected(t, h.fixture)
}

func TestService_ShutdownRejectsCommandsWhileWaitingForRequests(t *testing.T) {
	t.Parallel()

	h := newShutdownHarness(t)

	// The tick loop has stopped, so this request's reply never comes and it holds server shutdown
	// until the client gives up.
	replyCtx, giveUp := context.WithCancel(context.Background())
	defer giveUp()

	payload := testutils.SimpleCommand{Value: 1}
	cmd := &iscv1.Command{Name: payload.Name(), Address: h.fixture.world.address, Payload: payload.MarshalWire()}
	req := connect.NewRequest(&cardinalv1.SendCommandWithReplyRequest{
		Command:   cmd,
		EventName: testutils.SimpleEvent{}.Name(),
	})
	req.Header().Set(devPlayerIDHeader, shutdownTestPlayer)

	go func() { _, _ = h.client.SendCommandWithReply(replyCtx, req) }()

	awaitCommands(t, h.fixture) // The request now waits for its reply.

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- h.fixture.svc.shutdown(ctx) }()

	requireStreamUnavailable(t, h.ended) // Streams end just before server shutdown starts waiting.

	select {
	case err := <-done:
		require.FailNow(t, "shutdown returned while a request was still in flight", "%v", err)
	default:
	}

	requireCommandRejected(t, h.fixture)

	giveUp()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "shutdown did not finish after the last request ended")
	}
}

func TestService_ShutdownEndsStreamOpenedDuringIt(t *testing.T) {
	t.Parallel()

	h := newShutdownHarness(t)
	h.fixture.svc.server = nil // Keep serving, like a stream request that raced server shutdown.

	require.NoError(t, h.fixture.svc.shutdown(context.Background()))
	requireStreamUnavailable(t, h.ended)
	requireStreamUnavailable(t, h.open(t, "late-player"))
}

// -------------------------------------------------------------------------------------------------
// Harness
// -------------------------------------------------------------------------------------------------

type shutdownHarness struct {
	fixture *serviceFixture
	client  cardinalv1connect.CardinalServiceClient
	ctx     context.Context // Ends every client request at cleanup.
	ended   <-chan error    // The first stream's final error.
}

// newShutdownHarness serves fixture's service with NATS endpoints over h2c and opens one player's
// event stream.
func newShutdownHarness(t *testing.T) *shutdownHarness {
	t.Helper()

	fixture := newServiceFixture(t, testutils.NewRand(t), true)
	mux, err := fixture.svc.newMux(authenticatorDev{}.authenticate)
	require.NoError(t, err)

	server := httptest.NewUnstartedServer(mux)
	server.Config.Protocols = h2cProtocols()
	server.Start()
	t.Cleanup(server.Close)

	fixture.svc.server = server.Config

	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	transport := &http.Transport{Protocols: protocols}
	t.Cleanup(transport.CloseIdleConnections)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel) // Runs before server.Close.

	h := &shutdownHarness{
		fixture: fixture,
		client: cardinalv1connect.NewCardinalServiceClient(
			&http.Client{Transport: transport}, server.URL, connect.WithGRPC()),
		ctx: ctx,
	}
	h.ended = h.open(t, shutdownTestPlayer)

	return h
}

// open starts player's event stream, waits for its initial message, which the server sends after
// registering the stream, and returns the stream's final error once it ends.
func (h *shutdownHarness) open(t *testing.T, player string) <-chan error {
	t.Helper()

	req := connect.NewRequest(&cardinalv1.StartEventStreamRequest{})
	req.Header().Set(devPlayerIDHeader, player)
	stream, err := h.client.StartEventStream(h.ctx, req)
	require.NoError(t, err)
	require.True(t, stream.Receive(), "initial stream message: %v", stream.Err())

	ended := make(chan error, 1)

	go func() {
		for stream.Receive() {
		}

		ended <- stream.Err()
	}()

	return ended
}

func requireStreamUnavailable(t *testing.T, ended <-chan error) {
	t.Helper()

	select {
	case err := <-ended:
		require.Equal(t, connect.CodeUnavailable, connect.CodeOf(err), "%v", err)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "event stream still open after shutdown began")
	}
}

// requireCommandRejected checks that a command another shard sends now fails instead of being
// queued for a tick that will never run.
func requireCommandRejected(t *testing.T, fixture *serviceFixture) {
	t.Helper()

	peer := NewTestClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	payload := testutils.SimpleCommand{Value: 73}
	err := peer.SendCommand(ctx, fixture.world.address, fixture.world.address, payload.Name(), payload.MarshalWire())
	require.Error(t, err, "a command sent after the tick loop stopped must not be acknowledged")

	fixture.world.commands.Drain()
	queued, err := fixture.world.commands.Get(fixture.commandID)
	require.NoError(t, err)
	assert.Empty(t, queued)
}
