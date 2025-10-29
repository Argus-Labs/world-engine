package micro_test

import (
	"context"
	"testing"

	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
	"github.com/nats-io/nats.go"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/proto"
)

func TestMicro_Handler(t *testing.T) {
	t.Parallel()

	// Create a mock service address
	serviceAddr := micro.GetAddress("test-region", micro.RealmInternal, "test-org", "test-proj", "test-service")

	// Create a handler that returns a successful response
	successHandler := func(ctx context.Context, req *micro.Request) *micro.Response {
		// Return a success response with the service address
		return micro.NewSuccessResponse(req, serviceAddr)
	}

	// Create a handler that returns an error response
	errorHandler := func(ctx context.Context, req *micro.Request) *micro.Response {
		// Return an error response
		return micro.NewErrorResponse(req, eris.New("test error"), 3) // 3 = INVALID_ARGUMENT
	}

	// Test cases
	tests := []struct {
		name        string
		handler     micro.Handler
		requestID   string
		wantStatus  *status.Status
		wantPayload bool
	}{
		{
			name:    "success response",
			handler: successHandler,
			wantStatus: &status.Status{
				Code: 0, // OK
			},
			wantPayload: true,
		},
		{
			name:      "error response",
			handler:   errorHandler,
			requestID: "test-request-id",
			wantStatus: &status.Status{
				Code:    3, // INVALID_ARGUMENT
				Message: "test error",
			},
			wantPayload: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create a fake NATS message
			msg := &nats.Msg{
				Subject: "test.subject",
				Data:    []byte{},
			}

			// If requestID is specified, create a proper request
			if tt.requestID != "" {
				req := &microv1.Request{
					RequestId:      &tt.requestID,
					ServiceAddress: serviceAddr,
				}
				reqData, err := proto.Marshal(req)
				require.NoError(t, err)
				msg.Data = reqData
			}

			// Create telemetry for testing
			tel, err := telemetry.New(telemetry.Options{ServiceName: "test-handler"})
			require.NoError(t, err)

			// Process the message with our Handler
			responseBz, err := micro.HandleNATSMessage(t.Context(), msg, tt.handler, serviceAddr, tel.Tracer, zerolog.Nop())
			require.NoError(t, err)

			// Deserialize the response
			var response microv1.Response
			err = proto.Unmarshal(responseBz, &response)
			require.NoError(t, err)

			// Verify the response
			assert.Equal(t, tt.wantStatus.GetCode(), response.GetStatus().GetCode())
			assert.Equal(t, tt.wantStatus.GetMessage(), response.GetStatus().GetMessage())

			// Verify RequestID is passed through if provided
			if tt.requestID != "" {
				assert.NotNil(t, response.GetRequestId())
				assert.Equal(t, tt.requestID, response.GetRequestId())
			}

			// Verify ServiceAddress
			assert.NotNil(t, response.GetServiceAddress())
			assert.Equal(t, serviceAddr.GetRealm(), response.GetServiceAddress().GetRealm())
			assert.Equal(t, serviceAddr.GetOrganization(), response.GetServiceAddress().GetOrganization())
			assert.Equal(t, serviceAddr.GetProject(), response.GetServiceAddress().GetProject())
			assert.Equal(t, serviceAddr.GetServiceId(), response.GetServiceAddress().GetServiceId())

			// Verify payload is present or absent as expected
			if tt.wantPayload {
				assert.NotNil(t, response.GetPayload())
			} else {
				assert.Nil(t, response.GetPayload())
			}
		})
	}
}

func TestMicro_HandlerWithRequestDeserialization(t *testing.T) {
	t.Parallel()

	// Create a mock service address
	serviceAddr := micro.GetAddress("test-region", micro.RealmInternal, "test-org", "test-proj", "test-service")

	// Create a handler that verifies the request
	handler := func(ctx context.Context, req *micro.Request) *micro.Response {
		// Check that the request has the expected RequestID
		assert.Equal(t, "test-request-id", req.RequestID)

		// Return a success response
		return micro.NewSuccessResponse(req, nil)
	}

	// Create a proper request with payload
	requestID := "test-request-id"
	req := &microv1.Request{
		RequestId:      &requestID,
		ServiceAddress: serviceAddr,
	}

	reqData, err := proto.Marshal(req)
	require.NoError(t, err)

	// Create a fake NATS message
	msg := &nats.Msg{
		Subject: "test.subject",
		Data:    reqData,
	}

	// Create telemetry for testing
	tel, err := telemetry.New(telemetry.Options{ServiceName: "test-handler"})
	require.NoError(t, err)

	// Process the message with our Handler
	responseBz, err := micro.HandleNATSMessage(t.Context(), msg, handler, serviceAddr, tel.Tracer, zerolog.Nop())
	require.NoError(t, err)

	// Deserialize the response
	var response microv1.Response
	err = proto.Unmarshal(responseBz, &response)
	require.NoError(t, err)

	// Verify the response
	assert.Equal(t, int32(0), response.GetStatus().GetCode()) // OK

	// Verify RequestID is passed through
	assert.NotNil(t, response.GetRequestId())
	assert.Equal(t, requestID, response.GetRequestId())
}
