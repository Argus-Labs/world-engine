package cardinal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"connectrpc.com/validate"
	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_Subscriptions_RealHTTPServer stands up the real Cardinal ConnectRPC service behind the dev
// auth middleware and the validate/otel interceptors on an in-process HTTP test server, then drives
// it with a real connect client over HTTP. This exercises the full production path — HTTP transport,
// auth middleware, ConnectRPC framing, and net/http's handler-goroutine panic recovery — none of
// which the internal unit tests (which call handler methods directly) cover. With the TOCTOU fix, a
// closed stream surfaces as a clean connect.CodeFailedPrecondition over the wire; on the buggy code
// net/http would recover the panic and the client would see a connection/stream error instead.
func TestE2E_Subscriptions_RealHTTPServer(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)
	fixture := newServiceFixture(t, prng, false)

	otelInterceptor, err := otelconnect.NewInterceptor()
	require.NoError(t, err)
	validateInterceptor := validate.NewInterceptor()
	authMiddleware := authn.NewMiddleware(authenticatorDev{}.authenticate)

	cardinalPath, cardinalHandler := cardinalv1connect.NewCardinalServiceHandler(
		fixture.svc,
		connect.WithInterceptors(otelInterceptor, validateInterceptor),
	)
	mux := http.NewServeMux()
	mux.Handle(cardinalPath, authMiddleware.Wrap(cardinalHandler))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client := cardinalv1connect.NewCardinalServiceClient(server.Client(), server.URL)
	subscriptions := []*cardinalv1.EventSubscription{
		{Address: fixture.world.address, Events: []string{"currency", "combat"}},
	}

	t.Run("subscribe without open stream returns FailedPrecondition over HTTP", func(t *testing.T) {
		t.Parallel()
		req := connect.NewRequest(&cardinalv1.SubscribeEventsRequest{Subscriptions: subscriptions})
		req.Header().Set("X-Player-Id", "e2e-nostream-"+testutils.RandString(prng, 6))
		_, err := client.SubscribeEvents(context.Background(), req)
		require.Error(t, err)
		require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err),
			"expected clean FailedPrecondition over HTTP, got: %v", err)
		assert.Contains(t, err.Error(), "stream")
	})

	t.Run("unsubscribe without open stream returns FailedPrecondition over HTTP", func(t *testing.T) {
		t.Parallel()
		req := connect.NewRequest(&cardinalv1.UnsubscribeEventsRequest{Subscriptions: subscriptions})
		req.Header().Set("X-Player-Id", "e2e-nounsub-"+testutils.RandString(prng, 6))
		_, err := client.UnsubscribeEvents(context.Background(), req)
		require.Error(t, err)
		require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err),
			"expected clean FailedPrecondition over HTTP, got: %v", err)
	})

	t.Run("happy path: subscribe then unsubscribe over open stream", func(t *testing.T) {
		t.Parallel()
		email := "e2e-happy-" + testutils.RandString(prng, 6)

		sReq := connect.NewRequest(&cardinalv1.StartEventStreamRequest{Subscriptions: subscriptions})
		sReq.Header().Set("X-Player-Id", email)
		stream, err := client.StartEventStream(t.Context(), sReq)
		require.NoError(t, err)

		// Drain the initial empty response the server sends on stream open; run the receive loop in a
		// goroutine until the stream is cancelled.
		require.True(t, stream.Receive(), "initial response should be received; err: %v", stream.Err())
		require.NotNil(t, stream.Msg())
		go func() {
			for stream.Receive() {
			}
		}()

		// SubscribeEvents over the open stream should succeed (HTTP 200).
		subReq := connect.NewRequest(&cardinalv1.SubscribeEventsRequest{
			Subscriptions: []*cardinalv1.EventSubscription{
				{Address: fixture.world.address, Events: []string{"combat"}},
			},
		})
		subReq.Header().Set("X-Player-Id", email)
		_, err = client.SubscribeEvents(context.Background(), subReq)
		require.NoError(t, err, "SubscribeEvents over open stream should succeed over HTTP")

		// UnsubscribeEvents over the open stream should succeed.
		unsubReq := connect.NewRequest(&cardinalv1.UnsubscribeEventsRequest{
			Subscriptions: []*cardinalv1.EventSubscription{
				{Address: fixture.world.address, Events: []string{"currency"}},
			},
		})
		unsubReq.Header().Set("X-Player-Id", email)
		_, err = client.UnsubscribeEvents(context.Background(), unsubReq)
		require.NoError(t, err, "UnsubscribeEvents over open stream should succeed over HTTP")
	})

	t.Run("subscribe after stream closed returns FailedPrecondition over HTTP", func(t *testing.T) {
		t.Parallel()
		email := "e2e-close-" + testutils.RandString(prng, 6)
		streamCtx, cancelStream := context.WithCancel(context.Background())
		defer cancelStream()

		sReq := connect.NewRequest(&cardinalv1.StartEventStreamRequest{Subscriptions: subscriptions})
		sReq.Header().Set("X-Player-Id", email)
		stream, err := client.StartEventStream(streamCtx, sReq)
		require.NoError(t, err)

		// Drain initial response and keep the receive loop alive so the server detects the cancel via
		// ctx.Done rather than a receive error.
		require.True(t, stream.Receive(), "initial response should be received; err: %v", stream.Err())
		require.NotNil(t, stream.Msg())
		go func() {
			for stream.Receive() {
			}
		}()

		// Close the stream (client disconnect) and wait for the server's deferred removeSubscriber.
		cancelStream()
		require.Eventually(t, func() bool {
			fixture.svc.mu.RLock()
			defer fixture.svc.mu.RUnlock()
			_, ok := fixture.svc.subscribers[email]
			return !ok
		}, 2*time.Second, 5*time.Millisecond, "server should remove subscriber after stream close")

		// Now SubscribeEvents for the same user must return a clean FailedPrecondition over HTTP,
		// not a connection error (the recovered-panic bug produced a connection/stream tear-down).
		subReq := connect.NewRequest(&cardinalv1.SubscribeEventsRequest{Subscriptions: subscriptions})
		subReq.Header().Set("X-Player-Id", email)
		_, err = client.SubscribeEvents(context.Background(), subReq)
		require.Error(t, err)
		require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err),
			"after stream close, SubscribeEvents must return FailedPrecondition, got: %v", err)
		assert.Contains(t, err.Error(), "stream")
	})
}
