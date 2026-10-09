package transport

import (
	"context"

	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/micro"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// -------------------------------------------------------------------------------------------------
// Command handlers
// -------------------------------------------------------------------------------------------------

func (s *clientService) SendCommand(
	ctx context.Context,
	req *connect.Request[cardinalv1.SendCommandRequest],
) (*connect.Response[cardinalv1.SendCommandResponse], error) {
	select {
	case <-ctx.Done():
		return nil, connect.NewError(connect.CodeCanceled, eris.Wrap(ctx.Err(), "context cancelled"))
	default:
	}

	player := PlayerFromContext(ctx)
	assert.That(player != nil, "player should exist in authenticated request context")

	cmd := req.Msg.GetCommand()
	assert.That(cmd != nil, "command should have been validated")

	oteltrace.SpanFromContext(ctx).SetAttributes(semconv.EnduserID(player.ID), attrCommandName.String(cmd.GetName()))

	if micro.String(s.address) != micro.String(cmd.GetAddress()) {
		return nil, connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
	}

	if err := s.dispatch(ctx, cmd, PlayerSender(player.ID)); err != nil {
		return nil, dispatchError(err)
	}

	return connect.NewResponse(&cardinalv1.SendCommandResponse{}), nil
}

func (s *clientService) SendCommandWithReply(
	ctx context.Context,
	req *connect.Request[cardinalv1.SendCommandWithReplyRequest],
) (*connect.Response[cardinalv1.SendCommandWithReplyResponse], error) {
	player := PlayerFromContext(ctx)
	assert.That(player != nil, "player should exist in authenticated request context")

	cmd := req.Msg.GetCommand()
	assert.That(cmd != nil, "command should have been validated")

	span := oteltrace.SpanFromContext(ctx)
	span.SetAttributes(semconv.EnduserID(player.ID), attrCommandName.String(cmd.GetName()),
		attrEventName.String(req.Msg.GetEventName()))

	if micro.String(s.address) != micro.String(cmd.GetAddress()) {
		return nil, connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
	}

	// Register the waiter before dispatching: a handler that replies before returning would otherwise
	// publish before anyone waits.
	waiter := s.addReplyWaiter(player.ID, req.Msg.GetEventName())
	defer s.removeReplyWaiter(player.ID, req.Msg.GetEventName(), waiter)

	if err := s.dispatch(ctx, cmd, PlayerSender(player.ID)); err != nil {
		return nil, dispatchError(err)
	}

	// The span's duration is the round trip; this event marks where the enqueue ended and the wait
	// for the reply began. A cancelled wait ends the span with only this event and an error status.
	span.AddEvent("command enqueued")
	select {
	case <-ctx.Done():
		return nil, connect.NewError(connect.CodeCanceled, eris.Wrap(ctx.Err(), "waiting for reply event"))
	case <-s.stopped:
		return nil, connect.NewError(connect.CodeUnavailable, eris.Wrap(errStopping, "waiting for reply event"))
	case event := <-waiter:
		span.AddEvent("reply received")
		return connect.NewResponse(&cardinalv1.SendCommandWithReplyResponse{Event: event}), nil
	}
}

func (s *clientService) addReplyWaiter(playerID, eventName string) chan *iscv1.Event {
	waiter := make(chan *iscv1.Event, 1)

	s.mu.Lock()
	defer s.mu.Unlock()

	byPlayer := s.replyWaiters[eventName]
	if byPlayer == nil {
		byPlayer = make(map[string][]chan *iscv1.Event)
		s.replyWaiters[eventName] = byPlayer
	}
	byPlayer[playerID] = append(byPlayer[playerID], waiter)
	return waiter
}

func (s *clientService) removeReplyWaiter(playerID, eventName string, waiter chan *iscv1.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	byPlayer := s.replyWaiters[eventName]
	waiters := byPlayer[playerID]
	for i, current := range waiters {
		if current == waiter {
			byPlayer[playerID] = append(waiters[:i], waiters[i+1:]...)
			break
		}
	}
	if len(byPlayer[playerID]) == 0 {
		delete(byPlayer, playerID)
	}
	if len(byPlayer) == 0 {
		delete(s.replyWaiters, eventName)
	}
}
