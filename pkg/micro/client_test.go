package micro_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
)

// -------------------------------------------------------------------------------------------------
// Request integration test
// -------------------------------------------------------------------------------------------------
// Tests Client.Request using an in-process NATS server. Request is mostly glue code, so we test
// our logic (serialization, status-to-error mapping) rather than NATS internals. We intentionally
// skip cases like context timeouts or cancellation because those behaviors are already covered by
// NATS's own tests.
// -------------------------------------------------------------------------------------------------

func TestClient_Request(t *testing.T) {
	t.Parallel()

	prng := testutils.NewRand(t)
	client := micro.NewTestClient(t)
	testPayload := micro.RandServiceAddress(t, prng) // Use a service address as the payload

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		address := micro.RandServiceAddress(t, prng)
		endpoint := "happy"

		sub := newTestHandler(t, client, micro.Endpoint(address, endpoint), func(msg *nats.Msg) {
			request, err := micro.NewRequestFromNATSMsg(msg, address)
			require.NoError(t, err)

			response := micro.NewSuccessResponse(request, testPayload)
			payload, err := response.Bytes()
			require.NoError(t, err)

			msg.Respond(payload)
		})
		defer sub.Unsubscribe()

		// Flush ensures the subscription is registered on the server before we send a request.
		require.NoError(t, client.Flush())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		response, err := client.Request(ctx, address, endpoint, testPayload)

		require.NoError(t, err)
		require.NotNil(t, response)
		assert.Equal(t, int32(codes.OK), response.GetStatus().GetCode())
	})

	t.Run("handler returns error", func(t *testing.T) {
		t.Parallel()
		address := micro.RandServiceAddress(t, prng)
		endpoint := "app-error"

		sub := newTestHandler(t, client, micro.Endpoint(address, endpoint), func(msg *nats.Msg) {
			req, err := micro.NewRequestFromNATSMsg(msg, address)
			require.NoError(t, err)

			resp := micro.NewErrorResponse(req, errors.New("insufficient funds"), codes.InvalidArgument)
			responseBytes, err := resp.Bytes()
			require.NoError(t, err)

			msg.Respond(responseBytes)
		})
		defer sub.Unsubscribe()

		require.NoError(t, client.Flush())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		response, err := client.Request(ctx, address, endpoint, testPayload)

		require.Error(t, err)
		assert.Nil(t, response)
		assert.Contains(t, err.Error(), "insufficient funds")
	})

	t.Run("handler returns malformed response", func(t *testing.T) {
		t.Parallel()
		address := micro.RandServiceAddress(t, prng)
		endpoint := "malformed"

		sub := newTestHandler(t, client, micro.Endpoint(address, endpoint), func(msg *nats.Msg) {
			msg.Respond([]byte("not a valid protobuf"))
		})
		defer sub.Unsubscribe()

		require.NoError(t, client.Flush())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		response, err := client.Request(ctx, address, endpoint, testPayload)

		require.Error(t, err)
		assert.Nil(t, response)
		assert.Contains(t, err.Error(), "unmarshal")
	})
}

func newTestHandler(
	t *testing.T,
	client *micro.Client,
	address string,
	handler func(msg *nats.Msg),
) *nats.Subscription {
	sub, err := client.Subscribe(address, handler)
	require.NoError(t, err)
	return sub
}
