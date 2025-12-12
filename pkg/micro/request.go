package micro

import (
	"context"

	"buf.build/go/protovalidate"
	"github.com/argus-labs/world-engine/pkg/assert"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
	"github.com/nats-io/nats.go"
	"github.com/rotisserie/eris"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// Handler defines the signature for all service endpoint handlers.
// It guarantees that all responses follow the protobuf schema defined in response.proto.
type Handler func(ctx context.Context, req *Request) *Response

// Request represents an incoming service request with its metadata.
type Request struct {
	// Raw is the original NATS message.
	Raw *nats.Msg

	// RequestID is extracted from the request if available.
	RequestID string

	// ServiceAddress contains the service address information.
	ServiceAddress *microv1.ServiceAddress

	// Payload contains the decoded request payload, if any.
	Payload *anypb.Any
}

// Response represents a structured response that follows the protobuf schema.
type Response struct {
	// RequestID is copied from the request.
	RequestID string

	// ServiceAddress contains the service address information.
	ServiceAddress *microv1.ServiceAddress

	// Status contains the response status information.
	Status *spb.Status

	// Payload contains the response payload, if any.
	Payload *anypb.Any
}

// Bytes returns the response as a byte slice ready to be sent over NATS.
func (r *Response) Bytes() ([]byte, error) {
	resp := &microv1.Response{
		Status:         r.Status,
		ServiceAddress: r.ServiceAddress,
		Payload:        r.Payload,
	}

	if r.RequestID != "" {
		resp.RequestId = &r.RequestID
	}

	return proto.Marshal(resp)
}

// NewRequestFromNATSMsg converts a nats.Msg to a Request, parsing the payload if present.
func NewRequestFromNATSMsg(msg *nats.Msg, serviceAddr *microv1.ServiceAddress) (*Request, error) {
	req := &Request{
		Raw:            msg,
		ServiceAddress: serviceAddr,
	}

	if len(msg.Data) > 0 {
		var request microv1.Request
		if err := proto.Unmarshal(msg.Data, &request); err != nil {
			return nil, eris.Wrap(err, "failed to unmarshal request")
		}
		if err := protovalidate.Validate(&request); err != nil {
			return nil, eris.Wrap(err, "request validation failed")
		}
		if request.RequestId != nil {
			req.RequestID = request.GetRequestId()
		}
		req.Payload = request.GetPayload()
	}

	return req, nil
}

// NewSuccessResponse creates a successful response with optional payload.
func NewSuccessResponse(req *Request, payload proto.Message) *Response {
	var payloadAny *anypb.Any
	var err error

	if payload != nil {
		payloadAny, err = anypb.New(payload)
		if err != nil {
			// If we fail to create the Any payload, return an error response instead
			return NewErrorResponse(req, eris.New("failed to marshal payload"), codes.Internal)
		}
	}

	return &Response{
		RequestID:      req.RequestID,
		ServiceAddress: req.ServiceAddress,
		Status:         status.New(codes.OK, "").Proto(),
		Payload:        payloadAny,
	}
}

// NewErrorResponse creates an error response with the given error.
// The code parameter must not be codes.OK, as this function is only for error responses.
func NewErrorResponse(req *Request, err error, code codes.Code) *Response {
	assert.That(code != codes.OK, "NewErrorResponse called with codes.OK")

	var message string
	if err != nil {
		message = err.Error()
	} else {
		message = "Unknown error"
	}

	return &Response{
		RequestID:      req.RequestID,
		ServiceAddress: req.ServiceAddress,
		Status:         status.New(code, message).Proto(),
	}
}
