package micro_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/testutils"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

// -------------------------------------------------------------------------------------------------
// Request integration test
// -------------------------------------------------------------------------------------------------
// Tests Client.Request using an in-process NATS server. Request is mostly glue code, so we test
// our logic (serialization, status-to-error mapping) rather than NATS internals. We intentionally
// skip cases like context timeouts or cancellation because those behaviors are already covered by
// NATS's own tests.
// -------------------------------------------------------------------------------------------------

func TestClient_RequestAndSubscribe_NoDoubleUnsubscribe(t *testing.T) {
	t.Parallel()

	// Create a test NATS server
	natsTest := testutils.NewNATS(t)

	// Create a micro client with logger to capture warnings
	logger := zerolog.New(zerolog.NewTestWriter(t))
	client, err := micro.NewClient(
		micro.WithNATSConfig(micro.NATSConfig{
			Name: "test-double-unsub",
			URL:  natsTest.Server.ClientURL(),
		}),
		micro.WithLogger(logger),
	)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	serviceAddr := micro.GetAddress("test-region", micro.RealmInternal, "test-org", "test-proj", "test-service")

	testPayload := &microv1.ServiceAddress{
		Region:       "test-region",
		Realm:        microv1.ServiceAddress_REALM_INTERNAL,
		Organization: "test-org",
		Project:      "test-proj",
		ServiceId:    "test-service",
	}

	commandEndpoint := "command.double-unsub-test"
	eventEndpoint := "event.double-unsub-test"

	// Subscribe to the command endpoint and respond
	commandSub, err := natsTest.Client.Subscribe(micro.Endpoint(serviceAddr, commandEndpoint), func(msg *nats.Msg) {
		validationResponse := &microv1.Response{
			Status: &status.Status{Code: 0},
		}
		validationBytes, _ := proto.Marshal(validationResponse)
		msg.Respond(validationBytes)

		eventResponse := &microv1.Response{
			Status: &status.Status{Code: 0},
		}
		eventBytes, _ := proto.Marshal(eventResponse)
		natsTest.Client.Publish(micro.Endpoint(serviceAddr, eventEndpoint), eventBytes)
	})
	require.NoError(t, err)
	defer commandSub.Unsubscribe()

	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Call multiple times to ensure no subscription leaks or errors
	// If double unsubscribe bug exists, we'd see "Failed to unsubscribe" warnings in logs
	for i := 0; i < 10; i++ {
		msg, err := client.RequestAndSubscribe(ctx, serviceAddr, commandEndpoint, serviceAddr, eventEndpoint, testPayload)
		require.NoError(t, err)
		require.NotNil(t, msg)
	}
}

func TestClient_Request(t *testing.T) {
	t.Parallel()

	prng := testutils.NewRand(t)
	client := micro.NewTestClient2(t)
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

// -------------------------------------------------------------------------------------------------
// RequestAndSubscribe integration test
// -------------------------------------------------------------------------------------------------
// Tests Client.RequestAndSubscribe using an in-process NATS server. This method sends a request
// to one subject and waits for a response on a different subject (pub-sub pattern). Just like the
// above test, We only test the glue logic instead of NATS or protobuf code.
// -------------------------------------------------------------------------------------------------

func TestClient_RequestAndSubscribe(t *testing.T) {
	t.Parallel()

	prng := testutils.NewRand(t)
	client := micro.NewTestClient2(t)
	testPayload := micro.RandServiceAddress(t, prng)

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		address := micro.RandServiceAddress(t, prng)
		commandEndpoint := "command"
		eventEndpoint := "event"

		sub := newTestHandler(t, client, micro.Endpoint(address, commandEndpoint), func(msg *nats.Msg) {
			request, err := micro.NewRequestFromNATSMsg(msg, address)
			require.NoError(t, err)

			response1 := micro.NewSuccessResponse(request, testPayload)
			payload1, err := response1.Bytes()
			require.NoError(t, err)

			msg.Respond(payload1)

			response2 := micro.NewSuccessResponse(request, testPayload)
			payload2, err := response2.Bytes()
			require.NoError(t, err)
			client.Publish(micro.Endpoint(address, eventEndpoint), payload2)
		})
		defer sub.Unsubscribe()

		require.NoError(t, client.Flush())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		msg, err := client.RequestAndSubscribe(ctx, address, commandEndpoint, address, eventEndpoint, testPayload)

		require.NoError(t, err)
		require.NotNil(t, msg)

		var response microv1.Response
		err = proto.Unmarshal(msg.Data, &response)
		require.NoError(t, err)
		assert.Equal(t, int32(codes.OK), response.GetStatus().GetCode())
	})

	t.Run("send failure propagates", func(t *testing.T) {
		t.Parallel()
		address := micro.RandServiceAddress(t, prng)
		commandEndpoint := "command-fail"
		eventEndpoint := "event-fail"

		sub := newTestHandler(t, client, micro.Endpoint(address, commandEndpoint), func(msg *nats.Msg) {
			request, err := micro.NewRequestFromNATSMsg(msg, address)
			require.NoError(t, err)

			response := micro.NewErrorResponse(request, errors.New("validation failed"), codes.InvalidArgument)
			payload, err := response.Bytes()
			require.NoError(t, err)
			msg.Respond(payload)
		})
		defer sub.Unsubscribe()

		require.NoError(t, client.Flush())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		msg, err := client.RequestAndSubscribe(ctx, address, commandEndpoint, address, eventEndpoint, testPayload)

		require.Error(t, err)
		assert.Nil(t, msg)
		assert.Contains(t, err.Error(), "send failed")
		assert.Contains(t, err.Error(), "validation failed")
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
