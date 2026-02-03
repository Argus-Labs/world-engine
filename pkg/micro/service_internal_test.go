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
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc/codes"
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

	svc, client := newTestService(t, prng)
	testPayload := RandServiceAddress(t, prng) // Use ServiceAddress as test payload

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		endpoint := randEndpointName(prng)

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
		endpoint := randEndpointName(prng)

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
		assert.Contains(t, err.Error(), assert.AnError.Error())
	})

	t.Run("malformed request", func(t *testing.T) {
		t.Parallel()
		endpoint := randEndpointName(prng)

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
		endpoint := randEndpointName(prng)

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
		groupName := randEndpointName(prng)
		endpointName := randEndpointName(prng)

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
		Tracer: noop.NewTracerProvider().Tracer("test"),
	}
	client := NewTestClient2(t)
	address := RandServiceAddress(t, prng)

	svc, err := NewService(client, address, tel)
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Close() })

	return svc, client
}
