package micro

import (
	"context"
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/argus-labs/world-engine/pkg/testutils"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
	"github.com/nats-io/nats.go"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	otelcodes "go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// -------------------------------------------------------------------------------------------------
// Handler integration tests
// -------------------------------------------------------------------------------------------------
// Tests Service handler invocation using an in-process NATS server. We intentionally skip cases
// like context timeouts or cancellation because those behaviors are already covered by NATS's own
// tests.
// -------------------------------------------------------------------------------------------------

func TestService_Handler(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	testPayload := RandServiceAddress(t, prng) // Use ServiceAddress as test payload

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		rng := testutils.NewRand(t)
		svc, client := newTestService(t, rng)
		endpoint := randEndpointName(rng)

		err := svc.AddEndpoint(endpoint, func(_ context.Context, req *Request) *Response {
			return NewSuccessResponse(req, testPayload)
		})
		require.NoError(t, err)

		// Flush ensures the subscription is registered on the server before we send a request.
		require.NoError(t, client.Flush())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		resp, err := client.Request(ctx, svc.Address, endpoint, testPayload)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int32(codes.OK), resp.GetStatus().GetCode())
	})

	t.Run("handler returns error", func(t *testing.T) {
		t.Parallel()
		rng := testutils.NewRand(t)
		svc, client := newTestService(t, rng)
		endpoint := randEndpointName(rng)

		err := svc.AddEndpoint(endpoint, func(_ context.Context, req *Request) *Response {
			return NewErrorResponse(req, assert.AnError, codes.InvalidArgument)
		})
		require.NoError(t, err)
		require.NoError(t, client.Flush())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		resp, err := client.Request(ctx, svc.Address, endpoint, testPayload)

		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
		assert.Equal(t, assert.AnError.Error(), status.Convert(err).Message())
	})

	t.Run("malformed request", func(t *testing.T) {
		t.Parallel()
		rng := testutils.NewRand(t)
		svc, client := newTestService(t, rng)
		endpoint := randEndpointName(rng)

		handlerCalled := false
		err := svc.AddEndpoint(endpoint, func(_ context.Context, req *Request) *Response {
			handlerCalled = true
			return NewSuccessResponse(req, nil)
		})
		require.NoError(t, err)
		require.NoError(t, client.Flush())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Send raw invalid protobuf directly via NATS.
		msg, err := client.RequestWithContext(ctx, Endpoint(svc.Address, endpoint), []byte("not valid protobuf"))
		require.NoError(t, err)

		// Should get an error response, not a crash.
		var resp microv1.Response
		err = proto.Unmarshal(msg.Data, &resp)
		require.NoError(t, err)
		assert.Equal(t, int32(codes.Internal), resp.GetStatus().GetCode())
		assert.False(t, handlerCalled, "handler should not be called for malformed request")
	})

	t.Run("empty request", func(t *testing.T) {
		t.Parallel()
		rng := testutils.NewRand(t)
		svc, client := newTestService(t, rng)
		endpoint := randEndpointName(rng)

		var receivedReq *Request
		err := svc.AddEndpoint(endpoint, func(_ context.Context, req *Request) *Response {
			receivedReq = req
			return NewSuccessResponse(req, nil)
		})
		require.NoError(t, err)
		require.NoError(t, client.Flush())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Send empty data.
		msg, err := client.RequestWithContext(ctx, Endpoint(svc.Address, endpoint), []byte{})
		require.NoError(t, err)

		var resp microv1.Response
		err = proto.Unmarshal(msg.Data, &resp)
		require.NoError(t, err)
		assert.Equal(t, int32(codes.OK), resp.GetStatus().GetCode())
		assert.NotNil(t, receivedReq)
		assert.Empty(t, receivedReq.RequestID)
		assert.Nil(t, receivedReq.Payload)
	})

	t.Run("grouped endpoint", func(t *testing.T) {
		t.Parallel()
		rng := testutils.NewRand(t)
		svc, client := newTestService(t, rng)
		groupName := randEndpointName(rng)
		endpointName := randEndpointName(rng)

		group := svc.AddGroup(groupName)
		err := group.AddEndpoint(endpointName, func(_ context.Context, req *Request) *Response {
			return NewSuccessResponse(req, testPayload)
		})
		require.NoError(t, err)
		require.NoError(t, client.Flush())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Request using the full grouped endpoint name.
		resp, err := client.Request(ctx, svc.Address, groupName+"."+endpointName, testPayload)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int32(codes.OK), resp.GetStatus().GetCode())
	})

	// A reply subject outside _INBOX. could turn the response into a command the service never sent, so
	// the request is dropped unhandled and nothing is published there.
	t.Run("reply outside inbox", func(t *testing.T) {
		t.Parallel()
		rng := testutils.NewRand(t)
		svc, client := newTestService(t, rng)
		endpoint := randEndpointName(rng)

		handled := make(chan string, 2)
		err := svc.AddEndpoint(endpoint, func(_ context.Context, req *Request) *Response {
			handled <- req.RequestID
			return NewSuccessResponse(req, nil)
		})
		require.NoError(t, err)

		victim := CommandRoute{To: RandServiceAddress(t, rng), Name: "privileged", From: svc.Address}.Subject()
		spoofed, err := client.SubscribeSync(victim)
		require.NoError(t, err)
		require.NoError(t, client.Flush())

		spoof, err := proto.Marshal(&microv1.Request{RequestId: new("spoof")})
		require.NoError(t, err)
		require.NoError(t, client.PublishMsg(&nats.Msg{
			Subject: Endpoint(svc.Address, endpoint), Reply: victim, Data: spoof,
		}))

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, err = client.Request(ctx, svc.Address, endpoint, testPayload)
		require.NoError(t, err)

		assert.NotEqual(t, "spoof", <-handled, "handler saw the spoofing request")
		_, err = spoofed.NextMsg(100 * time.Millisecond)
		require.ErrorIs(t, err, nats.ErrTimeout, "service replied onto another service's command subject")
	})
}

// randEndpointName generates a random endpoint name for testing.
func randEndpointName(prng *rand.Rand) string {
	return "ep-" + strconv.FormatInt(prng.Int64(), 36)
}

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing endpoint registration
// -------------------------------------------------------------------------------------------------
// This test verifies the endpoint registration implementation correctness by applying random
// sequences of operations and comparing it against a regular Go map as the model.
// -------------------------------------------------------------------------------------------------

func TestService_RegisterModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const (
		opsMax             = 1 << 15 // 4096 iterations
		maxEndpoints       = 100     // limit unique endpoint names to increase collision chance
		maxGroups          = 10      // limit unique group names to increase collision chance
		opAddEndpoint      = "addEndpoint"
		opAddGroupEndpoint = "addGroupEndpoint"
	)

	// Randomize operation weights.
	operations := []string{opAddEndpoint, opAddGroupEndpoint}
	weights := testutils.RandOpWeights(prng, operations)

	impl, client := newTestService(t, prng)
	model := make(map[string]bool) // tracks registered endpoint names

	// Set the client's connection to nil so it doesn't actually create the NATS subscription.
	client.Conn = nil

	dummyHandler := func(_ context.Context, _ *Request) *Response { return nil }

	for range opsMax {
		op := testutils.RandWeightedOp(prng, weights)
		switch op {
		case opAddEndpoint:
			name := "endpoint-" + strconv.Itoa(prng.IntN(maxEndpoints))
			err := impl.AddEndpoint(name, dummyHandler)

			switch {
			case eris.Is(err, nats.ErrInvalidConnection):
				// Ignore connection errors from nil client - we only test registration logic.
			case model[name]:
				// Property: duplicate registration returns ErrEndpointAlreadyExists.
				require.ErrorIs(t, err, ErrEndpointAlreadyExists, "duplicate endpoint %q should error", name)
			default:
				// First registration should succeed (no other errors allowed).
				require.NoError(t, err, "first registration of %q should succeed", name)
				model[name] = true
			}

		case opAddGroupEndpoint:
			groupName := "group-" + strconv.Itoa(prng.IntN(maxGroups))
			endpointName := "endpoint-" + strconv.Itoa(prng.IntN(maxEndpoints))
			fullName := groupName + "." + endpointName

			group := impl.AddGroup(groupName)
			err := group.AddEndpoint(endpointName, dummyHandler)

			switch {
			case model[fullName]:
				// Property: duplicate group.endpoint returns ErrEndpointAlreadyExists.
				require.ErrorIs(t, err, ErrEndpointAlreadyExists, "duplicate group endpoint %q should error", fullName)
			case eris.Is(err, nats.ErrInvalidConnection):
				// Ignore connection errors from nil client - we only test registration logic.
			default:
				// First registration should succeed (no other errors allowed).
				require.NoError(t, err, "first registration of %q should succeed", fullName)
				model[fullName] = true
			}

		default:
			panic("unreachable")
		}
	}

	// Final state check: all successful registrations should be in endpoints map.
	for name := range model {
		_, exists := impl.endpoints[name]
		assert.True(t, exists, "endpoint %q should exist in service", name)
	}
}

func newTestService(t *testing.T, prng *rand.Rand) (*Service, *Client) {
	t.Helper()

	tel := &telemetry.Telemetry{
		Logger: zerolog.Nop(),
	}
	client := NewTestClient(t)
	address := RandServiceAddress(t, prng)

	svc, err := NewService(client, address, tel)
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Close() })

	return svc, client
}

// newRecordingTracer installs an in-memory span exporter as the global tracer provider for the
// duration of the test and returns it. Tests that call it must not run in parallel because it
// mutates global OTel state.
func newRecordingTracer(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	return exporter
}

// spansByName indexes exported spans by their name. If a name appears more than once the last
// span wins; tests that care about duplicates should inspect GetSpans directly.
func spansByName(exporter *tracetest.InMemoryExporter) map[string]tracetest.SpanStub {
	byName := make(map[string]tracetest.SpanStub)
	for _, s := range exporter.GetSpans() {
		byName[s.Name] = s
	}
	return byName
}

// requireSpans waits until at least want spans have been exported. The server-side handler runs
// on a NATS callback goroutine, so its deferred span.End may race the client receiving the reply.
func requireSpans(t *testing.T, exporter *tracetest.InMemoryExporter, want int) {
	t.Helper()
	require.Eventually(t, func() bool {
		return len(exporter.GetSpans()) >= want
	}, 2*time.Second, 5*time.Millisecond, "expected at least %d exported spans", want)
}

// -------------------------------------------------------------------------------------------------
// Handler span status (regression: outer span must mirror application-level failures)
// -------------------------------------------------------------------------------------------------
// Asserts the outer "handler.<name>" span mirrors the application-level gRPC status that the
// child "handler.execute" span already records.
// -------------------------------------------------------------------------------------------------

func TestService_HandlerSpanStatus(t *testing.T) {
	exporter := newRecordingTracer(t)

	t.Run("app-level failure sets both spans to Error", func(t *testing.T) {
		exporter.Reset()
		rng := testutils.NewRand(t)
		svc, client := newTestService(t, rng)
		endpoint := randEndpointName(rng)
		payload := RandServiceAddress(t, rng)

		require.NoError(t, svc.AddEndpoint(endpoint, func(_ context.Context, req *Request) *Response {
			return NewErrorResponse(req, eris.New("resource not found"), codes.NotFound)
		}))
		require.NoError(t, client.Flush())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_, err := client.Request(ctx, svc.Address, endpoint, payload)
		require.Error(t, err)
		require.Equal(t, codes.NotFound, status.Code(err))

		requireSpans(t, exporter, 2)
		byName := spansByName(exporter)

		outer, ok := byName["handler."+endpoint]
		require.True(t, ok, "outer span handler.%s missing", endpoint)
		inner, ok := byName["handler.execute"]
		require.True(t, ok, "inner span handler.execute missing")

		require.Equal(t, otelcodes.Error, outer.Status.Code,
			"outer span must be Error when the handler returns a non-OK status")
		require.Equal(t, "resource not found", outer.Status.Description,
			"outer span description must carry the app-level status message")
		require.Equal(t, otelcodes.Error, inner.Status.Code,
			"inner span must be Error when the handler returns a non-OK status")
		require.Equal(t, "resource not found", inner.Status.Description,
			"inner span description must carry the app-level status message")

		require.Equal(t, outer.SpanContext.SpanID(), inner.Parent.SpanID(),
			"inner span must be a child of the outer span")
	})

	t.Run("happy path sets both spans to Ok", func(t *testing.T) {
		exporter.Reset()
		rng := testutils.NewRand(t)
		svc, client := newTestService(t, rng)
		endpoint := randEndpointName(rng)
		payload := RandServiceAddress(t, rng)

		require.NoError(t, svc.AddEndpoint(endpoint, func(_ context.Context, req *Request) *Response {
			return NewSuccessResponse(req, payload)
		}))
		require.NoError(t, client.Flush())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		resp, err := client.Request(ctx, svc.Address, endpoint, payload)
		require.NoError(t, err)
		require.NotNil(t, resp)

		requireSpans(t, exporter, 2)
		byName := spansByName(exporter)

		outer, ok := byName["handler."+endpoint]
		require.True(t, ok, "outer span handler.%s missing", endpoint)
		inner, ok := byName["handler.execute"]
		require.True(t, ok, "inner span handler.execute missing")

		require.Equal(t, otelcodes.Ok, outer.Status.Code,
			"outer span must be Ok when the handler succeeds")
		require.Equal(t, otelcodes.Ok, inner.Status.Code,
			"inner span must be Ok when the handler succeeds")
	})
}
