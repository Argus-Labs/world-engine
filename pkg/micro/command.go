package micro

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"buf.build/go/protovalidate"
	"github.com/argus-labs/world-engine/pkg/telemetry/trace"
	"github.com/nats-io/nats.go"
	natsmicro "github.com/nats-io/nats.go/micro"
	"github.com/rotisserie/eris"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// An inter-shard command (ISC) is one service telling another to run a command. Its wire format has
// no envelope, so any NATS client in any language can send or serve one:
//
//	subject  <to>.isc.<name>.<from>
//	         us-west1.argus.rampage.gameplay-3.isc.plat_result.us-west1.argus.rampage.platform
//	body     the command's protobuf encoding, verbatim; empty when no field is set
//	headers  W3C trace context (traceparent, tracestate) when the sender is traced
//	reply    no headers: the receiver accepted the command
//	         Nats-Service-Error-Code (a gRPC code number) and Nats-Service-Error: it rejected it
//
// The sender's address is the last four subject tokens, which the NATS server can enforce: allow each
// service to publish only to "*.*.*.*.isc.*.<its own address>". With JWT auth, one template does it
// for every service, "*.*.*.*.isc.*.{{name()}}", if each user's name is its service address. Without
// such a permission the sender is only what the publisher claims.
//
// Replies need care, because a response permission lets a service publish to whatever reply subject a
// request names. A request whose reply subject is another service's command subject would make the
// receiver's reply arrive there as a command it never sent. So a receiver replies only to subjects
// under nats.InboxPrefix ("_INBOX."), the default for every NATS client, and drops any command or
// endpoint request that names another reply subject.

// replyOutsideInbox reports whether msg asks for a reply on a subject outside the inbox namespace.
func replyOutsideInbox(msg *nats.Msg) bool {
	return msg.Reply != "" && !strings.HasPrefix(msg.Reply, nats.InboxPrefix)
}

// commandEndpoint is the subject token between the receiver's address and the command name.
const commandEndpoint = "isc"

// commandSubjectTokens is the receiver's address, "isc", the name, and the sender's address.
const commandSubjectTokens = addressTokens + 2 + addressTokens

// commandName is the command name rule iscv1.Command enforces: one subject token.
var commandName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

// Command is an inter-shard command a service received.
type Command struct {
	Name    string          // The command name
	From    *ServiceAddress // The sending service, from the subject
	Payload []byte          // The command's protobuf encoding
}

// CommandHandler handles one received command. Returning nil accepts it. Returning an error rejects
// it; a gRPC status error sets the code the sender sees, any other error is codes.Unknown.
type CommandHandler func(ctx context.Context, cmd Command) error

// CommandRoute is what a command's subject says: who receives it, its name, and who sent it.
type CommandRoute struct {
	To   *ServiceAddress
	Name string
	From *ServiceAddress
}

// Subject returns the subject the command travels on.
func (r CommandRoute) Subject() string {
	return Endpoint(r.To, commandEndpoint+"."+r.Name+"."+String(r.From))
}

// ParseCommandSubject reads a command's route from its subject.
func ParseCommandSubject(subject string) (CommandRoute, error) {
	tokens := strings.Split(subject, ".")
	const endpoint, name = addressTokens, addressTokens + 1
	if len(tokens) != commandSubjectTokens || tokens[endpoint] != commandEndpoint ||
		!commandName.MatchString(tokens[name]) {
		return CommandRoute{}, eris.Errorf("not a command subject: %q", subject)
	}
	to, err := ParseAddress(strings.Join(tokens[:addressTokens], "."))
	if err != nil {
		return CommandRoute{}, eris.Wrap(err, "invalid receiver address")
	}
	from, err := ParseAddress(strings.Join(tokens[name+1:], "."))
	if err != nil {
		return CommandRoute{}, eris.Wrap(err, "invalid sender address")
	}
	if err := protovalidate.Validate(from); err != nil {
		return CommandRoute{}, eris.Wrap(err, "invalid sender address")
	}
	return CommandRoute{To: to, Name: tokens[name], From: from}, nil
}

// SendCommand sends a command and waits for the receiver to accept it. The timeout should be set in
// ctx. A rejection is returned as a gRPC status error carrying the receiver's code.
func (c *Client) SendCommand(ctx context.Context, from, to *ServiceAddress, name string, payload []byte) error {
	if !commandName.MatchString(name) {
		return eris.Errorf("invalid command name %q", name)
	}
	route := CommandRoute{To: to, Name: name, From: from}
	msg := &nats.Msg{Subject: route.Subject(), Data: payload, Header: nats.Header{}}
	otel.GetTextMapPropagator().Inject(ctx, headerCarrier(msg.Header))

	reply, err := c.RequestMsgWithContext(ctx, msg)
	if err != nil {
		return eris.Wrap(err, "failed to send command")
	}
	if code := reply.Header.Get(natsmicro.ErrorCodeHeader); code != "" {
		n, err := strconv.ParseUint(code, 10, 32)
		if err != nil || n == uint64(codes.OK) || n > uint64(codes.Unauthenticated) {
			return eris.Errorf("command rejected with invalid code %q: %s",
				code, reply.Header.Get(natsmicro.ErrorHeader))
		}
		return status.Error(codes.Code(n), reply.Header.Get(natsmicro.ErrorHeader))
	}
	return nil
}

// AddCommand serves the command name sent by any service to this one.
func (s *Service) AddCommand(name string, handler CommandHandler) error {
	if !commandName.MatchString(name) {
		return eris.Errorf("invalid command name %q", name)
	}
	key := commandEndpoint + "." + name
	if _, ok := s.endpoints[key]; ok {
		return eris.Wrap(ErrEndpointAlreadyExists, key)
	}

	sub, err := s.client.Subscribe(Endpoint(s.Address, key+".>"), func(msg *nats.Msg) {
		defer s.tel.RecoverAndFlush(true)
		ctx := otel.GetTextMapPropagator().Extract(context.Background(), headerCarrier(msg.Header))
		ctx, span := trace.New(ctx, "handler."+key,
			oteltrace.WithSpanKind(oteltrace.SpanKindConsumer),
			oteltrace.WithAttributes(attribute.String("nats.subject", msg.Subject)))
		defer span.End()
		log := s.tel.GetLoggerWithTrace(ctx, "service.handler").With().Str("endpoint", key).Logger()

		if replyOutsideInbox(msg) {
			span.SetStatus(otelcodes.Error, "reply subject outside the inbox namespace")
			log.Error().Str("reply", msg.Reply).Msg("command dropped: reply subject outside the inbox namespace")
			return
		}
		reply := nats.NewMsg(msg.Reply)
		if err := s.handleCommand(ctx, name, msg, handler); err != nil {
			st := status.Convert(err)
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, st.Message())
			log.Error().Err(err).Str("code", st.Code().String()).Msg("command rejected")
			reply.Header.Set(natsmicro.ErrorCodeHeader, strconv.FormatUint(uint64(st.Code()), 10))
			reply.Header.Set(natsmicro.ErrorHeader, st.Message())
		}
		if msg.Reply == "" {
			return // Published without waiting for a reply
		}
		if err := msg.RespondMsg(reply); err != nil {
			log.Error().Err(err).Msg("failed to send command reply over NATS")
		}
	})
	if err != nil {
		return eris.Wrapf(err, "failed to subscribe to command %s", name)
	}

	s.endpoints[key] = sub
	return nil
}

func (s *Service) handleCommand(ctx context.Context, name string, msg *nats.Msg, handler CommandHandler) error {
	route, err := ParseCommandSubject(msg.Subject)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	if route.Name != name {
		return status.Errorf(codes.InvalidArgument, "command %q arrived on the %q subscription", route.Name, name)
	}
	return handler(ctx, Command{Name: name, From: route.From, Payload: msg.Data})
}
