package shard

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/auth"
	"github.com/argus-labs/world-engine/pkg/micro"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This is the conformance suite for the client-facing protocol. Every runtime that serves it plugs
// a CommandSink in here; these tests pin the behavior a client is entitled to regardless of which
// runtime is behind the seam.

// sinkFunc adapts a function to CommandSink.
type sinkFunc func(ctx context.Context, cmd *iscv1.Command) error

func (f sinkFunc) Submit(ctx context.Context, cmd *iscv1.Command) error { return f(ctx, cmd) }

func testAddress(t *testing.T, id string) *micro.ServiceAddress {
	t.Helper()

	return micro.GetAddress("test-region", micro.RealmWorld, "test-org", "test-project", id)
}

func newTestServer(t *testing.T, sink CommandSink) *Server {
	t.Helper()

	server, err := NewServer(Config{
		Address:  testAddress(t, "shard-under-test"),
		Commands: sink,
		Logger:   zerolog.Nop(),
	})
	require.NoError(t, err)
	return server
}

// authedContext returns a context carrying an authenticated user, as the auth middleware would.
func authedContext(t *testing.T, userID string) context.Context {
	t.Helper()

	return authn.SetInfo(t.Context(), &auth.User{ID: userID})
}

func testCommand(address *micro.ServiceAddress) *iscv1.Command {
	return &iscv1.Command{
		Name:    "test_command",
		Address: address,
		Persona: &iscv1.Persona{Id: "whatever-the-client-claimed"},
		Payload: []byte("payload"),
	}
}

func TestNewServer_RequiresAddressAndSink(t *testing.T) {
	t.Parallel()

	sink := sinkFunc(func(context.Context, *iscv1.Command) error { return nil })

	_, err := NewServer(Config{Commands: sink})
	require.Error(t, err, "a server with no address cannot tell its own commands from another shard's")

	_, err = NewServer(Config{Address: testAddress(t, "s")})
	require.Error(t, err, "a server with no sink would accept commands and drop them")
}

// TestSendCommand_PersonaComesFromAuthenticatedUser is the protocol's core security property: the
// persona a command executes under is the authenticated identity, never what the client put in the
// request body.
func TestSendCommand_PersonaComesFromAuthenticatedUser(t *testing.T) {
	t.Parallel()

	var submitted *iscv1.Command
	server := newTestServer(t, sinkFunc(func(_ context.Context, cmd *iscv1.Command) error {
		submitted = cmd
		return nil
	}))

	_, err := server.SendCommand(
		authedContext(t, "the-real-user"),
		connect.NewRequest(&cardinalv1.SendCommandRequest{Command: testCommand(server.address)}),
	)
	require.NoError(t, err)

	require.NotNil(t, submitted)
	assert.Equal(t, "the-real-user", submitted.GetPersona().GetId(),
		"the client-supplied persona must be overwritten by the authenticated identity")
}

func TestSendCommand_RejectsAnotherShardsAddress(t *testing.T) {
	t.Parallel()

	called := false
	server := newTestServer(t, sinkFunc(func(context.Context, *iscv1.Command) error {
		called = true
		return nil
	}))

	_, err := server.SendCommand(
		authedContext(t, "user"),
		connect.NewRequest(&cardinalv1.SendCommandRequest{Command: testCommand(testAddress(t, "some-other-shard"))}),
	)
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.False(t, called, "a command for another shard must not reach the runtime")
}

func TestSendCommand_SinkErrorRejectsRequest(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, sinkFunc(func(context.Context, *iscv1.Command) error {
		return eris.New("runtime refused")
	}))

	_, err := server.SendCommand(
		authedContext(t, "user"),
		connect.NewRequest(&cardinalv1.SendCommandRequest{Command: testCommand(server.address)}),
	)
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestSendCommandWithReply_SynchronousSink is the regression test for reply-waiter ordering. A
// runtime that executes a command inside Submit publishes the reply event before Submit returns.
// If the waiter were registered after dispatch — as it once was — the event would land with nobody
// listening and this call would block until the client gave up.
//
// A ticked runtime never exposed the bug because its reply arrives a tick later, which is why the
// ordering has to be pinned here rather than left to each runtime to rediscover.
func TestSendCommandWithReply_SynchronousSink(t *testing.T) {
	t.Parallel()

	const replyEvent = "command_result"

	var server *Server
	server = newTestServer(t, sinkFunc(func(context.Context, *iscv1.Command) error {
		// Execute and publish the outcome before returning, as a transactional runtime does.
		return server.PublishEvent(Event{Name: replyEvent, Payload: []byte("result")})
	}))

	ctx, cancel := context.WithTimeout(authedContext(t, "user"), 2*time.Second)
	t.Cleanup(cancel)

	response, err := server.SendCommandWithReply(ctx, connect.NewRequest(
		&cardinalv1.SendCommandWithReplyRequest{
			Command:   testCommand(server.address),
			EventName: replyEvent,
		},
	))
	require.NoError(t, err, "reply published during Submit must still reach the waiter")
	require.Equal(t, replyEvent, response.Msg.GetEvent().GetName())
	require.Equal(t, []byte("result"), response.Msg.GetEvent().GetPayload())
}

// TestSendCommandWithReply_RejectedCommandDoesNotWait verifies a refused command fails immediately
// rather than leaving the client waiting for a reply that will never come.
func TestSendCommandWithReply_RejectedCommandDoesNotWait(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, sinkFunc(func(context.Context, *iscv1.Command) error { return nil }))

	ctx, cancel := context.WithTimeout(authedContext(t, "user"), 2*time.Second)
	t.Cleanup(cancel)

	_, err := server.SendCommandWithReply(ctx, connect.NewRequest(
		&cardinalv1.SendCommandWithReplyRequest{
			Command:   testCommand(testAddress(t, "some-other-shard")),
			EventName: "never_arrives",
		},
	))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestReplyWaiters_CleanedUpAfterUse verifies a completed call leaves no waiter behind, so a
// long-lived shard does not accumulate them.
func TestReplyWaiters_CleanedUpAfterUse(t *testing.T) {
	t.Parallel()

	const replyEvent = "cleanup_check"

	var server *Server
	server = newTestServer(t, sinkFunc(func(context.Context, *iscv1.Command) error {
		return server.PublishEvent(Event{Name: replyEvent})
	}))

	ctx, cancel := context.WithTimeout(authedContext(t, "user"), 2*time.Second)
	t.Cleanup(cancel)

	_, err := server.SendCommandWithReply(ctx, connect.NewRequest(
		&cardinalv1.SendCommandWithReplyRequest{
			Command:   testCommand(server.address),
			EventName: replyEvent,
		},
	))
	require.NoError(t, err)

	server.mu.RLock()
	defer server.mu.RUnlock()
	assert.Empty(t, server.replyWaiters, "a finished call must not leave its waiter registered")
}

// TestSubscribeEvents_RequiresEstablishedStream verifies subscriptions are refused until the client
// has a stream to deliver them on.
func TestSubscribeEvents_RequiresEstablishedStream(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, sinkFunc(func(context.Context, *iscv1.Command) error { return nil }))

	_, err := server.SubscribeEvents(authedContext(t, "user"), connect.NewRequest(
		&cardinalv1.SubscribeEventsRequest{},
	))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))

	_, err = server.UnsubscribeEvents(authedContext(t, "user"), connect.NewRequest(
		&cardinalv1.UnsubscribeEventsRequest{},
	))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

// TestPublishEvent_WithNoListeners verifies publishing into an empty shard is a no-op rather than
// an error, so a runtime need not track whether anyone is listening.
func TestPublishEvent_WithNoListeners(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, sinkFunc(func(context.Context, *iscv1.Command) error { return nil }))
	require.NoError(t, server.PublishEvent(Event{Name: "nobody_listening"}))
}

func TestHandler_MountsWithInterceptors(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, sinkFunc(func(context.Context, *iscv1.Command) error { return nil }))

	path, handler, err := server.Handler()
	require.NoError(t, err)
	assert.NotEmpty(t, path)
	assert.NotNil(t, handler)
}

// TestMatchesEvent pins the subscription matching rules. Clients rely on these exact semantics, so
// a runtime must not be in a position to reimplement them differently.
func TestMatchesEvent(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		subscription string
		eventName    string
		want         bool
	}{
		{subscription: "player_died", eventName: "player_died", want: true},
		{subscription: "player_died", eventName: "player_spawned", want: false},
		{subscription: "*", eventName: "anything", want: true},
		{subscription: ">", eventName: "anything", want: true},
		{subscription: "player.>", eventName: "player.died", want: true},
		{subscription: "player.>", eventName: "player.", want: true},
		{subscription: "player.>", eventName: "monster.died", want: false},
		{subscription: "player.>", eventName: "player", want: false},
		{subscription: "", eventName: "player_died", want: false},
	} {
		assert.Equal(t, tc.want, matchesEvent(tc.subscription, tc.eventName),
			"matchesEvent(%q, %q)", tc.subscription, tc.eventName)
	}
}
