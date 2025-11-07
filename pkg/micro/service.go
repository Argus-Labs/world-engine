package micro

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/argus-labs/world-engine/pkg/telemetry"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
	"github.com/nats-io/nats.go"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var (
	ErrEndpointAlreadyExists = eris.New("endpoint already exists")
)

// Service represents a micro service that can serve requests.
type Service struct {
	tel    *telemetry.Telemetry
	client *Client

	endpoints map[string]*nats.Subscription

	// Service address information
	Address      *ServiceAddress
	ProtoAddress *microv1.ServiceAddress
	Version      string // Version represents the service version.
}

// NewService creates a new service with the given NATS client, service address, and telemetry.
func NewService(client *Client, address *ServiceAddress, tel *telemetry.Telemetry) (*Service, error) {
	// Create protobuf service address - it's the same type now, so just use it directly
	protoAddress := address

	s := &Service{
		tel:          tel,
		client:       client,
		endpoints:    make(map[string]*nats.Subscription),
		Address:      address,
		ProtoAddress: protoAddress,
		Version:      runtime.Version(),
	}

	return s, nil
}

// Logger returns a logger for the service with service-specific context.
func (s *Service) Logger() *zerolog.Logger {
	logger := s.tel.GetLogger("service").With().
		Str("realm", realmToString(s.Address.Realm)).
		Str("organization", s.Address.Organization).
		Str("project", s.Address.Project).
		Str("service_id", s.Address.ServiceId).
		Logger()
	return &logger
}

// NATS returns the underlying NATS client.
// While it is possible to use the NATS client directly to publish and subscribe to endpoints,
// it is recommended to use the Service methods to handle messages received from NATS.
func (s *Service) NATS() *Client {
	return s.client
}

// AddGroup returns a helper struct that allows registering a group of endpoints with a common prefix.
// For example, all endpoints in the "message" group will be registered as "message.<endpoint_name>".
//
// Example:
//
//	messageGroup := svc.AddGroup("message")
//	messageGroup.AddEndpoint("send", handleMessageSend)   // -> "<service_address>.message.send"
//	messageGroup.AddEndpoint("delete", handleMessageDelete)   // -> "<service_address>.message.delete"
//	messageGroup.AddEndpoint("update", handleMessageUpdate)   // -> "<service_address>.message.update"
func (s *Service) AddGroup(name string) *ServiceEndpointGroup {
	return &ServiceEndpointGroup{
		service: s,
		group:   name,
	}
}

// AddEndpoint adds an endpoint to the service.
// The endpoint will be registered under the service's address as a prefix.
//
// Example:
//
//	svc.AddEndpoint("ping", handlePing)   // -> "<service_address>.ping"
func (s *Service) AddEndpoint(name string, handler Handler) error {
	if _, ok := s.endpoints[name]; ok {
		return eris.Wrap(ErrEndpointAlreadyExists, name)
	}

	sub, err := s.client.Subscribe(Endpoint(s.Address, name), func(msg *nats.Msg) {
		defer s.tel.RecoverAndFlush(true)
		// Extract parent context from incoming NATS headers.
		ctx := otel.GetTextMapPropagator().Extract(context.Background(), propagation.HeaderCarrier(msg.Header))

		// Start a span for the server-side request processing.
		ctx, span := s.tel.Tracer.Start(ctx, "handler."+name,
			trace.WithSpanKind(trace.SpanKindConsumer),
			trace.WithAttributes(attribute.String("nats.subject", msg.Subject)))
		defer span.End()

		// Use trace-aware logger.
		requestLogger := s.tel.GetLoggerWithTrace(ctx, "service.handler").With().Str("endpoint", name).Logger()

		// Start timing the request handler.
		start := time.Now()

		// Process the request.
		replyBz, err := HandleNATSMessage(ctx, msg, handler, s.ProtoAddress, s.tel.Tracer, requestLogger)

		// Calculate duration and add to span.
		duration := time.Since(start)
		span.SetAttributes(attribute.Int64("handler.duration_ms", duration.Milliseconds()))
		durationLogger := requestLogger.With().Int("duration_ms", int(duration.Milliseconds())).Logger()

		// If HandleNATSMessage returns an error, it means we failed to marshal the response. Here, we
		// create a generic internal server error response in place of the unmarshaleable response.
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			durationLogger.Error().Err(err).Msg("failed to marshal response payload")

			// Create a dummy request for error handling.
			errResp := NewErrorResponse(&Request{
				Raw:            msg,
				ServiceAddress: s.ProtoAddress,
			}, err, 13) // 13 = INTERNAL

			errRespBz, err := errResp.Bytes()
			if err != nil {
				durationLogger.Error().Err(err).Msg("failed to marshal error response")
				return
			}

			replyBz = errRespBz // Set response payload to the generic error response
		} else {
			span.SetStatus(codes.Ok, "")
		}

		// Respond to the message.
		if err := msg.Respond(replyBz); err != nil {
			durationLogger.Error().Err(err).Msg("failed to send response over NATS")
		} else {
			// Log successful network transmission (application success/failure already logged in HandleNATSMessage)
			durationLogger.Debug().Msg("response sent successfully")
		}
	})
	if err != nil {
		return eris.Wrap(err, fmt.Sprintf("failed to subscribe to endpoint %s", name))
	}

	s.endpoints[name] = sub

	return nil
}

// Close closes all the endpoints registered with the service.
func (s *Service) Close() error {
	var errs []error
	for _, sub := range s.endpoints {
		if err := sub.Unsubscribe(); err != nil {
			if !eris.Is(err, nats.ErrConnectionClosed) {
				s.Logger().Error().Err(err).Msg("failed to unsubscribe endpoint")
				errs = append(errs, err)
			}
		}
	}
	if len(errs) > 0 {
		return eris.New("one or more endpoints failed to unsubscribe")
	}
	return nil
}

// ---------------------------------------------------------------------------------------------------------------------
// Endpoint groups
// ---------------------------------------------------------------------------------------------------------------------

// ServiceEndpointGroup is a helper struct that allows registering a group of endpoints with a common prefix.
type ServiceEndpointGroup struct {
	service *Service
	group   string
}

func (g *ServiceEndpointGroup) AddEndpoint(name string, handler Handler) error {
	return g.service.AddEndpoint(g.group+"."+name, handler)
}
