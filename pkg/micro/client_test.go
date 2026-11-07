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
	"google.golang.org/grpc/status"
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

func TestClient_Request(t *testing.T) {
	t.Parallel()

	prng := testutils.NewRand(t)
	client := micro.NewTestClient(t)
	testPayload := micro.RandServiceAddress(t, prng) // Use a service address as the payload

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		rng := testutils.NewRand(t)
		address := micro.RandServiceAddress(t, rng)
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

	requestStatusError := func(
		t *testing.T,
		buildResponse func(req *micro.Request) *micro.Response,
		wantCode codes.Code,
		checkStatus func(*testing.T, *status.Status),
	) {
		t.Helper()

		rng := testutils.NewRand(t)
		address := micro.RandServiceAddress(t, rng)
		endpoint := "status-error"

		sub := newTestHandler(t, client, micro.Endpoint(address, endpoint), func(msg *nats.Msg) {
			req, err := micro.NewRequestFromNATSMsg(msg, address)
			//nolint:testifylint // require would FailNow in this NATS callback goroutine, leaving the request to time out.
			if !assert.NoError(t, err) {
				return
			}
			resp := buildResponse(req)
			b, err := resp.Bytes()
			//nolint:testifylint // require would FailNow in this NATS callback goroutine, leaving the request to time out.
			if !assert.NoError(t, err) {
				return
			}
			msg.Respond(b)
		})
		defer sub.Unsubscribe()

		require.NoError(t, client.Flush())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		response, err := client.Request(ctx, address, endpoint, testPayload)
		require.Error(t, err)
		assert.Nil(t, response)
		assert.Equal(t, wantCode, status.Code(err))
		checkStatus(t, status.Convert(err))
	}

	t.Run("handler returns error", func(t *testing.T) {
		t.Parallel()
		requestStatusError(t,
			func(req *micro.Request) *micro.Response {
				return micro.NewErrorResponse(req, errors.New("insufficient funds"), codes.InvalidArgument)
			},
			codes.InvalidArgument,
			func(t *testing.T, st *status.Status) {
				assert.Equal(t, "insufficient funds", st.Message())
			},
		)
	})

	t.Run("handler returns malformed response", func(t *testing.T) {
		t.Parallel()
		rng := testutils.NewRand(t)
		address := micro.RandServiceAddress(t, rng)
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

	t.Run("preserves gRPC status code", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name    string
			code    codes.Code
			message string
		}{
			{"NotFound", codes.NotFound, "thing not found"},
			{"AlreadyExists", codes.AlreadyExists, "already exists"},
			{"Unavailable", codes.Unavailable, "temporarily unavailable"},
			{"ResourceExhausted", codes.ResourceExhausted, "quota exceeded"},
			{"PermissionDenied", codes.PermissionDenied, "access denied"},
			{"Unimplemented", codes.Unimplemented, "not supported"},
			{"NotFoundEmptyMessage", codes.NotFound, ""},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				requestStatusError(t,
					func(req *micro.Request) *micro.Response {
						return micro.NewErrorResponse(req, errors.New(tc.message), tc.code)
					},
					tc.code,
					func(t *testing.T, st *status.Status) {
						assert.Equal(t, tc.message, st.Message())
					},
				)
			})
		}
	})

	t.Run("preserves status details", func(t *testing.T) {
		t.Parallel()
		detail := &micro.ServiceAddress{Project: "detail-proj", ServiceId: "detail-svc"}
		requestStatusError(t,
			func(req *micro.Request) *micro.Response {
				st, err := status.New(codes.FailedPrecondition, "state conflict").WithDetails(detail)
				require.NoError(t, err)
				return &micro.Response{
					RequestID:      req.RequestID,
					ServiceAddress: req.ServiceAddress,
					Status:         st.Proto(),
				}
			},
			codes.FailedPrecondition,
			func(t *testing.T, st *status.Status) {
				details := st.Details()
				require.Len(t, details, 1)
				sa, ok := details[0].(*microv1.ServiceAddress)
				require.True(t, ok, "expected *microv1.ServiceAddress detail, got %T", details[0])
				assert.True(t, proto.Equal(sa, detail))
			},
		)
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
	client := micro.NewTestClient(t)
	testPayload := micro.RandServiceAddress(t, prng)

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		rng := testutils.NewRand(t)
		address := micro.RandServiceAddress(t, rng)
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
		rng := testutils.NewRand(t)
		address := micro.RandServiceAddress(t, rng)
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
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
		assert.Contains(t, err.Error(), "send failed")
		assert.Contains(t, err.Error(), "validation failed")
	})

	t.Run("no double unsubscribe errors", func(t *testing.T) {
		t.Parallel()
		rng := testutils.NewRand(t)
		address := micro.RandServiceAddress(t, rng)
		commandEndpoint := "command-double-unsub"
		eventEndpoint := "event-double-unsub"

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

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		for range rng.IntN(9) + 2 {
			msg, err := client.RequestAndSubscribe(ctx, address, commandEndpoint, address, eventEndpoint, testPayload)
			require.NoError(t, err)
			require.NotNil(t, msg)
		}
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
