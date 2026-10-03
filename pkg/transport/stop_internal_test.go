package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTransport_StopWaitsForHandlerReply checks the order Stop runs in for a service that handles
// commands at once: a handler still running when Stop begins finishes, its reply to another shard is
// sent, and a command arriving meanwhile is rejected as unavailable.
func TestTransport_StopWaitsForHandlerReply(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	fixtureB := newTransportFixture(t, prng, true)

	address := RandServiceAddress(prng)
	tr, err := New(Options{
		Address:   address,
		AuthMode:  AuthModeDev,
		Telemetry: &telemetry.Telemetry{Logger: zerolog.Nop()},
	})
	require.NoError(t, err)
	entered, release := make(chan struct{}), make(chan struct{})
	tr.Handle(testutils.SimpleCommand{}.Name(), func(ctx context.Context, cmd *iscv1.Command) error {
		close(entered)
		<-release
		tr.Enqueue(ctx, fixtureB.address, decodeSimpleCommand(t, cmd))
		tr.Flush()
		return nil
	})
	require.NoError(t, tr.Start(NewTestClient(t)))

	send := func(value int) error {
		_, err := tr.clients.SendCommand(transportTestContext("user"), connect.NewRequest(&cardinalv1.SendCommandRequest{
			Command: &iscv1.Command{
				Name:    testutils.SimpleCommand{}.Name(),
				Address: address,
				Persona: &iscv1.Persona{Id: "client-provided-persona"},
				Payload: testutils.SimpleCommand{Value: value}.MarshalWire(),
			},
		}))
		return err
	}

	firstErr := make(chan error, 1)
	go func() { firstErr <- send(1) }()
	<-entered

	stopErr := make(chan error, 1)
	go func() { stopErr <- tr.Stop(context.Background()) }()
	require.Eventually(t, func() bool {
		tr.inflight.mu.Lock()
		defer tr.inflight.mu.Unlock()
		return tr.inflight.closed
	}, 5*time.Second, time.Millisecond)

	err = send(2)
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err), "command during Stop: %v", err)
	select {
	case <-stopErr:
		t.Fatal("Stop returned while a handler was still running")
	default:
	}

	close(release)
	require.NoError(t, <-firstErr)
	require.NoError(t, <-stopErr)

	// Stop returned only after the reply was sent, so one take sees it.
	cmds := fixtureB.received.take()
	require.Len(t, cmds, 1)
	assert.Equal(t, testutils.SimpleCommand{Value: 1}, decodeSimpleCommand(t, cmds[0]))
}

// TestTransport_StopEndsEventStream checks that Stop ends an open event stream, so the app's HTTP server
// shuts down at once instead of waiting for the stream until its deadline.
func TestTransport_StopEndsEventStream(t *testing.T) {
	t.Parallel()
	fixture := newTransportFixture(t, testutils.NewRand(t), true)

	path, handler, err := fixture.tr.Handler()
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	server := httptest.NewUnstartedServer(mux)
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()

	client := cardinalv1connect.NewCardinalServiceClient(server.Client(), server.URL)
	req := connect.NewRequest(&cardinalv1.StartEventStreamRequest{})
	req.Header().Set("X-Email", "holder")
	stream, err := client.StartEventStream(context.Background(), req)
	require.NoError(t, err)
	require.True(t, stream.Receive(), "no initial message: %v", stream.Err())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	require.NoError(t, fixture.tr.Stop(ctx))
	require.NoError(t, server.Config.Shutdown(ctx))
	assert.Less(t, time.Since(start), 2*time.Second, "shutdown waited on the open stream")

	assert.False(t, stream.Receive())
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(stream.Err()))
}

// TestTransport_FlushAfterStop checks that commands flushed after Stop are dropped without starting a
// sender.
func TestTransport_FlushAfterStop(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	fixtureA := newTransportFixture(t, prng, true)
	fixtureB := newTransportFixture(t, prng, true)
	require.NoError(t, fixtureA.tr.Stop(context.Background()))

	fixtureA.tr.Enqueue(context.Background(), fixtureB.address, testutils.SimpleCommand{Value: 1})
	fixtureA.tr.Flush()

	link := fixtureA.tr.interShard
	link.mu.Lock()
	senders := len(link.senders)
	link.mu.Unlock()
	assert.Zero(t, senders, "a sender started after Stop")
	time.Sleep(100 * time.Millisecond)
	assert.Empty(t, fixtureB.received.take())
}

// TestTransport_PublishWhileStreamsClose checks that publishing while streams close never writes to a
// stream connect-go has already finished. Run with -race.
func TestTransport_PublishWhileStreamsClose(t *testing.T) {
	t.Parallel()
	fixture := newTransportFixture(t, testutils.NewRand(t), false)

	path, handler, err := fixture.tr.Handler()
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	server := httptest.NewUnstartedServer(mux)
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client := cardinalv1connect.NewCardinalServiceClient(server.Client(), server.URL)

	done := make(chan struct{})
	var publisher sync.WaitGroup
	publisher.Go(func() {
		for {
			select {
			case <-done:
				return
			default:
				fixture.tr.Publish(context.Background(), testutils.SimpleEvent{Value: 1}, "")
			}
		}
	})
	defer publisher.Wait()
	defer close(done)

	for i := range 100 {
		ctx, cancel := context.WithCancel(context.Background())
		req := connect.NewRequest(&cardinalv1.StartEventStreamRequest{Subscriptions: []*cardinalv1.EventSubscription{
			{Address: fixture.address, Events: []string{"*"}},
		}})
		req.Header().Set("X-Email", "user-"+strconv.Itoa(i))
		stream, err := client.StartEventStream(ctx, req)
		require.NoError(t, err)
		stream.Receive()
		time.Sleep(time.Millisecond) // Let some events reach the stream before it closes
		cancel()
	}
}
