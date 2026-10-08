package micro

import (
	"context"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/testutils"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// -------------------------------------------------------------------------------------------------
// NewRequestFromNATSMsg request_id preservation
// -------------------------------------------------------------------------------------------------
// request_id is a client-supplied correlation field that the framework is expected to echo from
// request to response on both success and error paths. These unit tests verify the parsing contract
// that makes that possible: when the request can be partially parsed, the request_id must survive
// even if validation (or anything else downstream) fails.
//
// All prng access happens before t.Parallel() is called, so parallel subtests never touch the
// (non-concurrency-safe) *rand.Rand concurrently.
// -------------------------------------------------------------------------------------------------

// invalidServiceAddress returns a ServiceAddress that fails protovalidate's constraints
// (REALM_UNSPECIFIED violates the `not_in: [0]` rule), while still being a valid wire message.
func invalidServiceAddress() *microv1.ServiceAddress {
	return &microv1.ServiceAddress{
		Region: "us-west1",
		Realm:  microv1.ServiceAddress_REALM_UNSPECIFIED,
	}
}

// newWireRequestMsg marshals a microv1.Request into a *nats.Msg ready for NewRequestFromNATSMsg.
func newWireRequestMsg(t *testing.T, request *microv1.Request) *nats.Msg {
	t.Helper()
	data, err := proto.Marshal(request)
	require.NoError(t, err)
	return &nats.Msg{Data: data}
}

func TestNewRequestFromNATSMsg_RequestID(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)
	serverAddr := RandServiceAddress(t, prng)
	happyPayloadAddr := RandServiceAddress(t, prng)
	const requestID = "abc-123"

	t.Run("happy path preserves request_id and payload", func(t *testing.T) {
		t.Parallel()
		payloadAny, err := anypb.New(happyPayloadAddr)
		require.NoError(t, err)

		msg := newWireRequestMsg(t, &microv1.Request{
			RequestId:      proto.String(requestID),
			ServiceAddress: serverAddr,
			Payload:        payloadAny,
		})

		req, err := NewRequestFromNATSMsg(msg, serverAddr)
		require.NoError(t, err)
		require.NotNil(t, req)
		assert.Equal(t, requestID, req.RequestID)
		assert.True(t, proto.Equal(payloadAny, req.Payload))
	})

	t.Run("validation failure returns req with request_id preserved", func(t *testing.T) {
		t.Parallel()
		// ServiceAddress with REALM_UNSPECIFIED fails protovalidate (not_in: [0]), so the
		// request is rejected even though request_id parses cleanly.
		msg := newWireRequestMsg(t, &microv1.Request{
			RequestId:      proto.String(requestID),
			ServiceAddress: invalidServiceAddress(),
		})

		req, err := NewRequestFromNATSMsg(msg, serverAddr)
		require.Error(t, err)
		require.NotNil(t, req, "req must be non-nil on validation failure so callers can echo request_id")
		assert.Equal(t, requestID, req.RequestID, "request_id must be preserved on validation failure")
		assert.Nil(t, req.Payload, "payload must not be populated when validation fails")
		assert.Contains(t, err.Error(), "validation failed")
	})

	t.Run("unmarshal failure returns nil req", func(t *testing.T) {
		t.Parallel()
		msg := &nats.Msg{Data: []byte("not valid protobuf")}

		req, err := NewRequestFromNATSMsg(msg, serverAddr)
		require.Error(t, err)
		assert.Nil(t, req, "req must be nil when the protobuf cannot be parsed at all")
		assert.Contains(t, err.Error(), "unmarshal")
	})

	t.Run("empty data yields req with no request_id", func(t *testing.T) {
		t.Parallel()
		msg := &nats.Msg{Data: nil}

		req, err := NewRequestFromNATSMsg(msg, serverAddr)
		require.NoError(t, err)
		require.NotNil(t, req)
		assert.Empty(t, req.RequestID)
		assert.Nil(t, req.Payload)
	})

	t.Run("missing request_id yields empty RequestID", func(t *testing.T) {
		t.Parallel()
		msg := newWireRequestMsg(t, &microv1.Request{
			ServiceAddress: serverAddr,
		})

		req, err := NewRequestFromNATSMsg(msg, serverAddr)
		require.NoError(t, err)
		require.NotNil(t, req)
		assert.Empty(t, req.RequestID)
	})
}

// -------------------------------------------------------------------------------------------------
// Service error responses echo request_id end-to-end
// -------------------------------------------------------------------------------------------------
// These tests exercise the full AddEndpoint pipeline (handleNATSMessage -> outer error fallback)
// through an in-process NATS server, confirming that request_id is echoed on error responses for
// every reachable trigger path and dropped only when the request genuinely cannot be parsed.
//
// Each subtest owns its own Service so that the (non-concurrency-safe) endpoints map is never
// shared across parallel subtests, and all prng access happens before t.Parallel() is called.
// -------------------------------------------------------------------------------------------------

func TestService_ErrorResponsePreservesRequestID(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)
	const requestID = "abc-123"

	t.Run("validation failure echoes request_id and skips handler", func(t *testing.T) {
		svc, client := newTestService(t, prng)
		endpoint := randEndpointName(prng)
		t.Parallel()

		handlerCalled := false
		require.NoError(t, svc.AddEndpoint(endpoint, func(_ context.Context, req *Request) *Response {
			handlerCalled = true
			return NewSuccessResponse(req, nil)
		}))
		require.NoError(t, client.Flush())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// request_id is present; the embedded ServiceAddress is invalid, so protovalidate rejects
		// the request before the handler runs.
		reqBytes, err := proto.Marshal(&microv1.Request{
			RequestId:      proto.String(requestID),
			ServiceAddress: invalidServiceAddress(),
		})
		require.NoError(t, err)

		msg, err := client.RequestWithContext(ctx, Endpoint(svc.Address, endpoint), reqBytes)
		require.NoError(t, err)

		var resp microv1.Response
		require.NoError(t, proto.Unmarshal(msg.Data, &resp))
		assert.Equal(t, requestID, resp.GetRequestId(),
			"error response must echo request_id when request validation fails")
		assert.Equal(t, int32(codes.Internal), resp.GetStatus().GetCode())
		assert.False(t, handlerCalled, "handler must not be called when request validation fails")
	})

	t.Run("marshal failure echoes request_id", func(t *testing.T) {
		svc, client := newTestService(t, prng)
		endpoint := randEndpointName(prng)
		t.Parallel()

		// Handler returns a hand-crafted response whose payload carries invalid UTF-8 in the
		// Any.type_url field, causing Response.Bytes() (proto.Marshal) to fail. This exercises the
		// marshal-failure path: the request parsed successfully (request_id is known) but the
		// response could not be serialized. The outer fallback must reuse the parsed request so the
		// error response still carries request_id.
		require.NoError(t, svc.AddEndpoint(endpoint, func(_ context.Context, req *Request) *Response {
			return &Response{
				RequestID:      req.RequestID,
				ServiceAddress: req.ServiceAddress,
				Status:         status.New(codes.OK, "").Proto(),
				Payload: &anypb.Any{
					TypeUrl: "\xff\xfe\xfd", // invalid UTF-8 -> proto.Marshal rejects string field
				},
			}
		}))
		require.NoError(t, client.Flush())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Valid request (passes validation) so the handler is reached; only the response marshal fails.
		reqBytes, err := proto.Marshal(&microv1.Request{
			RequestId:      proto.String(requestID),
			ServiceAddress: svc.Address,
		})
		require.NoError(t, err)

		msg, err := client.RequestWithContext(ctx, Endpoint(svc.Address, endpoint), reqBytes)
		require.NoError(t, err)

		var resp microv1.Response
		require.NoError(t, proto.Unmarshal(msg.Data, &resp))
		assert.Equal(t, requestID, resp.GetRequestId(),
			"error response must echo request_id when response marshal fails")
		assert.Equal(t, int32(codes.Internal), resp.GetStatus().GetCode())
	})

	t.Run("happy path echoes request_id", func(t *testing.T) {
		svc, client := newTestService(t, prng)
		endpoint := randEndpointName(prng)
		testPayload := RandServiceAddress(t, prng)
		t.Parallel()

		require.NoError(t, svc.AddEndpoint(endpoint, func(_ context.Context, req *Request) *Response {
			return NewSuccessResponse(req, testPayload)
		}))
		require.NoError(t, client.Flush())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		reqBytes, err := proto.Marshal(&microv1.Request{
			RequestId:      proto.String(requestID),
			ServiceAddress: svc.Address,
		})
		require.NoError(t, err)

		msg, err := client.RequestWithContext(ctx, Endpoint(svc.Address, endpoint), reqBytes)
		require.NoError(t, err)

		var resp microv1.Response
		require.NoError(t, proto.Unmarshal(msg.Data, &resp))
		assert.Equal(t, requestID, resp.GetRequestId(), "success response must echo request_id")
		assert.Equal(t, int32(codes.OK), resp.GetStatus().GetCode())
	})

	t.Run("unparseable request yields empty request_id", func(t *testing.T) {
		svc, client := newTestService(t, prng)
		endpoint := randEndpointName(prng)
		t.Parallel()

		handlerCalled := false
		require.NoError(t, svc.AddEndpoint(endpoint, func(_ context.Context, req *Request) *Response {
			handlerCalled = true
			return NewSuccessResponse(req, nil)
		}))
		require.NoError(t, client.Flush())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Garbage bytes that proto.Unmarshal cannot decode: request_id is not reliably available,
		// so the fallback uses a fresh Request and the response must carry no request_id.
		msg, err := client.RequestWithContext(ctx, Endpoint(svc.Address, endpoint), []byte("not valid protobuf"))
		require.NoError(t, err)

		var resp microv1.Response
		require.NoError(t, proto.Unmarshal(msg.Data, &resp))
		assert.Empty(t, resp.GetRequestId(),
			"request_id must be absent when the request could not be parsed at all")
		assert.Equal(t, int32(codes.Internal), resp.GetStatus().GetCode())
		assert.False(t, handlerCalled, "handler must not be called for an unparseable request")
	})
}
