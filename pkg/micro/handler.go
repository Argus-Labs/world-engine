package micro

import (
	"context"

	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
	"github.com/nats-io/nats.go"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/genproto/googleapis/rpc/status"
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
	Status *status.Status

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

// HandleNATSMessage converts a NATS message to a Request, calls the handler, and converts
// the Response back to bytes for NATS. This is used internally by the Service.
func HandleNATSMessage(
	ctx context.Context,
	msg *nats.Msg,
	handler Handler,
	serviceAddr *microv1.ServiceAddress,
	tracer trace.Tracer,
	logger zerolog.Logger,
) ([]byte, error) {
	// Create child span for handler execution
	ctx, span := tracer.Start(ctx, "handler.execute",
		trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()
	// Create a request from the incoming NATS message.
	req := &Request{
		Raw:            msg,
		ServiceAddress: serviceAddr,
	}

	// Attempt to decode the request payload if it exists.
	if len(msg.Data) > 0 {
		var request microv1.Request
		if err := proto.Unmarshal(msg.Data, &request); err == nil {
			if request.RequestId != nil {
				req.RequestID = request.GetRequestId()
			}
			req.Payload = request.GetPayload()
		}
	}
	span.SetAttributes(attribute.String("request.id", req.RequestID))

	// Enhanced request logging with more context.
	reqLogger := logger.With().Str("request_id", req.RequestID).Logger()
	reqLogger.Info().Msg("request received")

	// Call the handler to get a response.
	resp := handler(ctx, req)

	// Log based on response status to catch application-level errors.
	statusCode := resp.Status.GetCode()
	statusMessage := resp.Status.GetMessage()

	// Record application status in span
	span.SetAttributes(attribute.Int("status.code", int(statusCode)))
	if statusCode != 0 {
		span.RecordError(eris.New(statusMessage))
		span.SetStatus(codes.Error, statusMessage)
		reqLogger.Error().Int32("code", statusCode).Str("message", statusMessage).Msg("request failed")
	} else {
		span.SetStatus(codes.Ok, "")
		reqLogger.Info().Msg("request processed successfully")
	}

	// Convert response to bytes.
	respBytes, err := resp.Bytes()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, eris.Wrap(err, "failed to marshal response")
	}

	return respBytes, nil
}

// NewSuccessResponse creates a successful response with optional payload.
func NewSuccessResponse(req *Request, payload proto.Message) *Response {
	var payloadAny *anypb.Any
	var err error

	if payload != nil {
		payloadAny, err = anypb.New(payload)
		if err != nil {
			// If we fail to create the Any payload, return an error response instead
			return NewErrorResponse(req, eris.New("failed to marshal payload"), 0)
		}
	}

	return &Response{
		RequestID:      req.RequestID,
		ServiceAddress: req.ServiceAddress,
		Status: &status.Status{
			Code: 0, // OK
		},
		Payload: payloadAny,
	}
}

// NewErrorResponse creates an error response with the given error.
func NewErrorResponse(req *Request, err error, code int32) *Response {
	var message string
	if err != nil {
		message = err.Error()
	} else {
		message = "Unknown error"
	}

	// If code is not provided, use internal error code
	if code == 0 {
		code = 13 // Internal error
	}

	return &Response{
		RequestID:      req.RequestID,
		ServiceAddress: req.ServiceAddress,
		Status: &status.Status{
			Code:    code,
			Message: message,
		},
	}
}
