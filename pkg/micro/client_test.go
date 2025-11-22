package micro_test

import (
	"context"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/micro/testutils"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestClient_RequestAndSubscribe(t *testing.T) {
	t.Parallel()

	// Create a test NATS server
	natsTest := testutils.NewNATS(t)

	// Create a micro client
	client, err := micro.NewTestClient(natsTest.Server.ClientURL())
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	// Create a mock service address
	serviceAddr := micro.GetAddress("test-region", micro.RealmInternal, "test-org", "test-proj", "test-service")

	// Test payload
	testPayload := &microv1.ServiceAddress{
		Region:       "test-region",
		Realm:        microv1.ServiceAddress_REALM_INTERNAL,
		Organization: "test-org",
		Project:      "test-proj",
		ServiceId:    "test-service",
	}

	t.Run("successful request-and-subscribe flow", func(t *testing.T) {
		t.Parallel()
		commandEndpoint := "command.buy-item"
		eventSubject := "event.item-purchased"

		// Subscribe to the command endpoint and respond with validation + event
		commandSub, err := natsTest.Client.Subscribe(micro.Endpoint(serviceAddr, commandEndpoint), func(msg *nats.Msg) {
			// Unmarshal the request
			var req microv1.Request
			err := proto.Unmarshal(msg.Data, &req)
			require.NoError(t, err)

			// Step 1: Respond with immediate validation success (via msg.Respond)
			validationResponse := &microv1.Response{
				ServiceAddress: serviceAddr,
				Status: &status.Status{
					Code:    0, // Success
					Message: "",
				},
			}
			validationBytes, err := proto.Marshal(validationResponse)
			require.NoError(t, err)

			err = msg.Respond(validationBytes)
			require.NoError(t, err)

			// Step 2: Publish the actual result event to the event subject
			anyPayload, err := anypb.New(testPayload)
			require.NoError(t, err)

			eventResponse := &microv1.Response{
				ServiceAddress: serviceAddr,
				Payload:        anyPayload,
				Status: &status.Status{
					Code:    0, // Success
					Message: "",
				},
			}

			eventBytes, err := proto.Marshal(eventResponse)
			require.NoError(t, err)

			err = natsTest.Client.Publish(eventSubject, eventBytes)
			require.NoError(t, err)
		})
		require.NoError(t, err)
		defer commandSub.Unsubscribe()

		// Give the subscription time to be ready
		time.Sleep(50 * time.Millisecond)

		// Send the request with context timeout
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		response, err := client.RequestAndSubscribe(ctx, serviceAddr, commandEndpoint, testPayload, eventSubject)
		require.NoError(t, err)
		require.NotNil(t, response)

		// Verify the response
		assert.Equal(t, int32(0), response.GetStatus().GetCode())
		assert.NotNil(t, response.GetPayload())
	})

	t.Run("error response from server", func(t *testing.T) {
		t.Parallel()
		commandEndpoint := "command.invalid-action"
		eventSubject := "event.invalid-action-result"

		// Subscribe to the command endpoint and respond with validation success, then error event
		commandSub, err := natsTest.Client.Subscribe(micro.Endpoint(serviceAddr, commandEndpoint), func(msg *nats.Msg) {
			// Step 1: Validation passes (command accepted)
			validationResponse := &microv1.Response{
				ServiceAddress: serviceAddr,
				Status: &status.Status{
					Code:    0,
					Message: "",
				},
			}
			validationBytes, err := proto.Marshal(validationResponse)
			require.NoError(t, err)

			err = msg.Respond(validationBytes)
			require.NoError(t, err)

			// Step 2: But processing fails (publish error event)
			errorResponse := &microv1.Response{
				ServiceAddress: serviceAddr,
				Status: &status.Status{
					Code:    3, // INVALID_ARGUMENT
					Message: "invalid action requested",
				},
			}

			errorBytes, err := proto.Marshal(errorResponse)
			require.NoError(t, err)

			err = natsTest.Client.Publish(eventSubject, errorBytes)
			require.NoError(t, err)
		})
		require.NoError(t, err)
		defer commandSub.Unsubscribe()

		// Give the subscription time to be ready
		time.Sleep(50 * time.Millisecond)

		// Send the request with context timeout
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		response, err := client.RequestAndSubscribe(ctx, serviceAddr, commandEndpoint, testPayload, eventSubject)

		// Should get an error because status code is non-zero
		require.Error(t, err)
		assert.Nil(t, response)
		assert.Contains(t, err.Error(), "invalid action requested")
	})

	t.Run("context timeout", func(t *testing.T) {
		t.Parallel()
		commandEndpoint := "command.slow-action"
		eventSubject := "event.slow-action-result"

		// Subscribe to the command but don't respond
		commandSub, err := natsTest.Client.Subscribe(micro.Endpoint(serviceAddr, commandEndpoint), func(msg *nats.Msg) {
			// Don't publish any response - simulate a timeout
		})
		require.NoError(t, err)
		defer commandSub.Unsubscribe()

		// Give the subscription time to be ready
		time.Sleep(50 * time.Millisecond)

		// Send the request with a very short timeout via context
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		response, err := client.RequestAndSubscribe(ctx, serviceAddr, commandEndpoint, testPayload, eventSubject)

		// Should get a timeout error
		require.Error(t, err)
		assert.Nil(t, response)
		assert.Contains(t, err.Error(), "context")
	})

	t.Run("context cancellation", func(t *testing.T) {
		t.Parallel()
		commandEndpoint := "command.cancel-action"
		eventSubject := "event.cancel-action-result"

		// Subscribe to the command but delay the response
		commandSub, err := natsTest.Client.Subscribe(micro.Endpoint(serviceAddr, commandEndpoint), func(msg *nats.Msg) {
			time.Sleep(500 * time.Millisecond)
			// By the time we get here, context should be cancelled
		})
		require.NoError(t, err)
		defer commandSub.Unsubscribe()

		// Give the subscription time to be ready
		time.Sleep(50 * time.Millisecond)

		// Create a context we can cancel
		ctx, cancel := context.WithCancel(context.Background())

		// Cancel the context after a short delay
		go func() {
			time.Sleep(100 * time.Millisecond)
			cancel()
		}()

		response, err := client.RequestAndSubscribe(ctx, serviceAddr, commandEndpoint, testPayload, eventSubject)

		// Should get a context cancelled error
		require.Error(t, err)
		assert.Nil(t, response)
		assert.Contains(t, err.Error(), "context")
	})

	t.Run("invalid payload", func(t *testing.T) {
		t.Parallel()
		commandEndpoint := "command.test"
		eventSubject := "event.test-result"

		// Try to send a nil payload (should fail during anypb.New)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		response, err := client.RequestAndSubscribe(ctx, serviceAddr, commandEndpoint, nil, eventSubject)

		// Should get an error about creating Any payload
		require.Error(t, err)
		assert.Nil(t, response)
	})

	t.Run("custom timeouts with options", func(t *testing.T) {
		t.Parallel()
		commandEndpoint := "command.custom-timeout"
		eventSubject := "event.custom-timeout-result"

		// Subscribe to the command endpoint and respond
		commandSub, err := natsTest.Client.Subscribe(micro.Endpoint(serviceAddr, commandEndpoint), func(msg *nats.Msg) {
			var req microv1.Request
			err := proto.Unmarshal(msg.Data, &req)
			if err != nil {
				return
			}

			validationResponse := &microv1.Response{
				Status: &status.Status{Code: 0},
			}
			validationBytes, _ := proto.Marshal(validationResponse)
			msg.Respond(validationBytes)

			eventResponse := &microv1.Response{
				Payload: req.GetPayload(),
				Status:  &status.Status{Code: 0},
			}
			eventBytes, _ := proto.Marshal(eventResponse)
			natsTest.Client.Publish(eventSubject, eventBytes)
		})
		require.NoError(t, err)
		defer commandSub.Unsubscribe()

		time.Sleep(50 * time.Millisecond)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Use custom timeouts
		response, err := client.RequestAndSubscribe(ctx, serviceAddr, commandEndpoint, testPayload, eventSubject,
			micro.WithRequestTimeout(5*time.Second),
			micro.WithResponseTimeout(5*time.Second),
		)

		require.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, int32(0), response.GetStatus().GetCode())
	})
}

func TestClient_Request(t *testing.T) {
	t.Parallel()

	// Create a test NATS server
	natsTest := testutils.NewNATS(t)

	// Create a micro client
	client, err := micro.NewTestClient(natsTest.Server.ClientURL())
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	// Create a mock service address
	serviceAddr := micro.GetAddress("test-region", micro.RealmInternal, "test-org", "test-proj", "test-service")

	testPayload := &microv1.ServiceAddress{
		Region:       "test-region",
		Realm:        microv1.ServiceAddress_REALM_INTERNAL,
		Organization: "test-org",
		Project:      "test-proj",
		ServiceId:    "test-service",
	}

	t.Run("successful request", func(t *testing.T) {
		t.Parallel()
		endpoint := "test.endpoint"

		// Subscribe to the endpoint and respond
		sub, err := natsTest.Client.Subscribe(micro.Endpoint(serviceAddr, endpoint), func(msg *nats.Msg) {
			var req microv1.Request
			err := proto.Unmarshal(msg.Data, &req)
			if err != nil {
				return
			}

			response := &microv1.Response{
				Payload: req.GetPayload(),
				Status:  &status.Status{Code: 0},
			}
			responseBytes, _ := proto.Marshal(response)
			msg.Respond(responseBytes)
		})
		require.NoError(t, err)
		defer sub.Unsubscribe()

		time.Sleep(50 * time.Millisecond)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		response, err := client.Request(ctx, serviceAddr, endpoint, testPayload)

		require.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, int32(0), response.GetStatus().GetCode())
	})

	t.Run("error response", func(t *testing.T) {
		t.Parallel()
		endpoint := "test.error"

		// Subscribe and respond with error
		sub, err := natsTest.Client.Subscribe(micro.Endpoint(serviceAddr, endpoint), func(msg *nats.Msg) {
			response := &microv1.Response{
				Status: &status.Status{
					Code:    1,
					Message: "test error",
				},
			}
			responseBytes, _ := proto.Marshal(response)
			msg.Respond(responseBytes)
		})
		require.NoError(t, err)
		defer sub.Unsubscribe()

		time.Sleep(50 * time.Millisecond)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		response, err := client.Request(ctx, serviceAddr, endpoint, testPayload)

		require.Error(t, err)
		assert.Nil(t, response)
		assert.Contains(t, err.Error(), "test error")
	})

	t.Run("invalid payload", func(t *testing.T) {
		t.Parallel()
		endpoint := "test.endpoint"

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		response, err := client.Request(ctx, serviceAddr, endpoint, nil)

		require.Error(t, err)
		assert.Nil(t, response)
	})

	t.Run("context timeout", func(t *testing.T) {
		t.Parallel()
		endpoint := "test.timeout"

		// Subscribe but don't respond
		sub, err := natsTest.Client.Subscribe(micro.Endpoint(serviceAddr, endpoint), func(msg *nats.Msg) {
			// Don't respond
		})
		require.NoError(t, err)
		defer sub.Unsubscribe()

		time.Sleep(50 * time.Millisecond)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		response, err := client.Request(ctx, serviceAddr, endpoint, testPayload)

		require.Error(t, err)
		assert.Nil(t, response)
	})
}

func TestClient_RequestAndSubscribe_WithLogger(t *testing.T) {
	t.Parallel()

	// Create a test NATS server
	natsTest := testutils.NewNATS(t)

	// Create a micro client with logger
	logger := zerolog.New(zerolog.NewTestWriter(t))
	client, err := micro.NewClient(
		micro.WithNATSConfig(micro.NATSConfig{
			Name: "test-client-with-logger",
			URL:  natsTest.Server.ClientURL(),
		}),
		micro.WithLogger(logger),
	)
	require.NoError(t, err)
	defer client.Close()

	// Create a mock service address
	serviceAddr := micro.GetAddress("test-region", micro.RealmInternal, "test-org", "test-proj", "test-service")

	testPayload := &microv1.ServiceAddress{
		Region:       "test-region",
		Realm:        microv1.ServiceAddress_REALM_INTERNAL,
		Organization: "test-org",
		Project:      "test-proj",
		ServiceId:    "test-service",
	}

	commandEndpoint := "command.test-logging"
	eventSubject := "event.test-logging-result"

	// Subscribe and respond
	commandSub, err := natsTest.Client.Subscribe(micro.Endpoint(serviceAddr, commandEndpoint), func(msg *nats.Msg) {
		// Step 1: Validation response
		validationResponse := &microv1.Response{
			ServiceAddress: serviceAddr,
			Status: &status.Status{
				Code: 0,
			},
		}
		validationBytes, err := proto.Marshal(validationResponse)
		require.NoError(t, err)

		err = msg.Respond(validationBytes)
		require.NoError(t, err)

		// Step 2: Event response
		anyPayload, err := anypb.New(testPayload)
		require.NoError(t, err)

		eventResponse := &microv1.Response{
			ServiceAddress: serviceAddr,
			Payload:        anyPayload,
			Status: &status.Status{
				Code: 0,
			},
		}

		eventBytes, err := proto.Marshal(eventResponse)
		require.NoError(t, err)

		err = natsTest.Client.Publish(eventSubject, eventBytes)
		require.NoError(t, err)
	})
	require.NoError(t, err)
	defer commandSub.Unsubscribe()

	// Give the subscription time to be ready
	time.Sleep(50 * time.Millisecond)

	// Send the request with context timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	response, err := client.RequestAndSubscribe(ctx, serviceAddr, commandEndpoint, testPayload, eventSubject)
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, int32(0), response.GetStatus().GetCode())
}
