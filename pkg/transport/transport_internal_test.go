package transport

import (
	"context"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// SendCommand smoke tests
// -------------------------------------------------------------------------------------------------
// Verifies that the ConnectRPC command handler names the player as sender, hands commands to the registered
// handler, and rejects commands addressed to the wrong shard or with no handler.
// -------------------------------------------------------------------------------------------------

func TestTransport_SendCommand(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newTransportFixture(t, prng, false)

		payload := testutils.SimpleCommand{Value: prng.IntN(1_000_000)}
		userID := testutils.RandString(prng, 8)
		cmdPb := &iscv1.Command{
			Name:    payload.Name(),
			Address: fixture.address,
			Payload: payload.MarshalWire(),
		}

		_, err := fixture.tr.clients.SendCommand(
			transportTestContext(userID),
			connect.NewRequest(&cardinalv1.SendCommandRequest{Command: cmdPb}),
		)
		require.NoError(t, err)

		cmds, senders := fixture.received.takeWithSenders()
		require.Len(t, cmds, 1)
		assert.Equal(t, payload, decodeSimpleCommand(t, cmds[0]))
		assert.Equal(t, PlayerSender(userID), senders[0])
	})

	t.Run("wrong address rejected", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newTransportFixture(t, prng, false)

		payload := testutils.SimpleCommand{Value: 42}
		cmdPb := &iscv1.Command{
			Name:    payload.Name(),
			Address: RandServiceAddress(prng),
			Payload: payload.MarshalWire(),
		}

		_, err := fixture.tr.clients.SendCommand(
			transportTestContext(testutils.RandString(prng, 8)),
			connect.NewRequest(&cardinalv1.SendCommandRequest{Command: cmdPb}),
		)
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.Contains(t, err.Error(), "address")
		assert.Empty(t, fixture.received.take())
	})

	t.Run("unhandled command rejected", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newTransportFixture(t, prng, false)

		cmdPb := &iscv1.Command{
			Name:    "unhandled_command",
			Address: fixture.address,
		}

		_, err := fixture.tr.clients.SendCommand(
			transportTestContext(testutils.RandString(prng, 8)),
			connect.NewRequest(&cardinalv1.SendCommandRequest{Command: cmdPb}),
		)
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.Contains(t, err.Error(), "unregistered command")
	})
}

// -------------------------------------------------------------------------------------------------
// Publish smoke tests
// -------------------------------------------------------------------------------------------------
// Verifies that publishing an event encodes the payload and delivers it to registered reply waiters
// with round-trip integrity.
// -------------------------------------------------------------------------------------------------

func TestTransport_Publish(t *testing.T) {
	t.Parallel()

	t.Run("reply waiter", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newTransportFixture(t, prng, false)

		payload := testutils.SimpleEvent{Value: prng.Int()}
		playerID := testutils.RandString(prng, 8)
		waiter := fixture.tr.clients.addReplyWaiter(playerID, payload.Name())
		defer fixture.tr.clients.removeReplyWaiter(playerID, payload.Name(), waiter)

		fixture.tr.Publish(context.Background(), payload, "")

		var eventPb *iscv1.Event
		select {
		case eventPb = <-waiter:
		case <-time.After(5 * time.Second):
			require.FailNow(t, "timed out waiting for published event")
		}
		assert.Equal(t, payload.Name(), eventPb.GetName())
		decoded, err := testutils.SimpleEvent{}.UnmarshalWire(eventPb.GetPayload())
		require.NoError(t, err)
		assert.Equal(t, payload, decoded)
	})

	t.Run("reply published inside the handler", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		address := RandServiceAddress(prng)
		tr, err := New(Options{
			Address:   address,
			AuthMode:  AuthModeDev,
			Telemetry: &telemetry.Telemetry{Logger: zerolog.Nop()},
		})
		require.NoError(t, err)

		// A handler that processes the command at once, as a non-ECS service may, and replies before
		// returning.
		reply := testutils.SimpleEvent{Value: prng.Int()}
		tr.Handle(testutils.SimpleCommand{}.Name(), func(ctx context.Context, _ *iscv1.Command, _ Sender) error {
			tr.Publish(ctx, reply, "")
			return nil
		})

		ctx, cancel := context.WithTimeout(transportTestContext(testutils.RandString(prng, 8)), 2*time.Second)
		defer cancel()
		res, err := tr.clients.SendCommandWithReply(ctx, connect.NewRequest(&cardinalv1.SendCommandWithReplyRequest{
			Command: &iscv1.Command{
				Name:    testutils.SimpleCommand{}.Name(),
				Address: address,
				Payload: testutils.SimpleCommand{Value: 1}.MarshalWire(),
			},
			EventName: reply.Name(),
		}))
		require.NoError(t, err, "the reply was published before the waiter existed")
		decoded, err := testutils.SimpleEvent{}.UnmarshalWire(res.Msg.GetEvent().GetPayload())
		require.NoError(t, err)
		assert.Equal(t, reply, decoded)
	})
}

// -------------------------------------------------------------------------------------------------
// SendCommandWithReply reply routing
// -------------------------------------------------------------------------------------------------
// Reply waiters are keyed by player and event name. A targeted reply must resolve only the
// recipient's pending request, so one player never receives another player's private reply. A
// broadcast is visible to every player, so it resolves every pending request for the event name.
// -------------------------------------------------------------------------------------------------

type replyResult struct {
	reply any
	err   error
}

// newReplyRoutingTransport returns a transport whose SimpleCommand handler signals each command it
// receives. SendCommandWithReply registers its waiter before dispatching, so a signal means the request
// is waiting for its reply.
func newReplyRoutingTransport(t *testing.T) (*Transport, *micro.ServiceAddress, <-chan struct{}) {
	t.Helper()
	address := RandServiceAddress(testutils.NewRand(t))
	tr, err := New(Options{
		Address:   address,
		AuthMode:  AuthModeDev,
		Telemetry: &telemetry.Telemetry{Logger: zerolog.Nop()},
	})
	require.NoError(t, err)
	waiting := make(chan struct{})
	tr.Handle(testutils.SimpleCommand{}.Name(), func(context.Context, *iscv1.Command, Sender) error {
		waiting <- struct{}{}
		return nil
	})
	return tr, address, waiting
}

// startReplyRequest sends a SimpleCommand with reply as playerID in the background and returns once the
// request is waiting for its reply. The channel yields the decoded reply or the call's error.
func startReplyRequest(
	t *testing.T, tr *Transport, address *micro.ServiceAddress, waiting <-chan struct{}, playerID string,
) <-chan replyResult {
	t.Helper()
	results := make(chan replyResult, 1)
	go func() {
		ctx, cancel := context.WithTimeout(transportTestContext(playerID), 5*time.Second)
		defer cancel()
		res, err := tr.clients.SendCommandWithReply(ctx, connect.NewRequest(&cardinalv1.SendCommandWithReplyRequest{
			Command: &iscv1.Command{
				Name:    testutils.SimpleCommand{}.Name(),
				Address: address,
				Payload: testutils.SimpleCommand{}.MarshalWire(),
			},
			EventName: testutils.SimpleEvent{}.Name(),
		}))
		if err != nil {
			results <- replyResult{err: err}
			return
		}
		reply, err := testutils.SimpleEvent{}.UnmarshalWire(res.Msg.GetEvent().GetPayload())
		results <- replyResult{reply: reply, err: err}
	}()
	select {
	case <-waiting:
	case res := <-results:
		require.FailNow(t, "request returned before it was dispatched", "error: %v", res.err)
	}
	return results
}

func TestTransport_SendCommandWithReply_RoutesReplyByRecipient(t *testing.T) {
	t.Parallel()

	t.Run("targeted reply resolves only the recipient's request", func(t *testing.T) {
		t.Parallel()
		tr, address, waiting := newReplyRoutingTransport(t)
		alice := startReplyRequest(t, tr, address, waiting, "alice")
		bob := startReplyRequest(t, tr, address, waiting, "bob")

		tr.Publish(t.Context(), testutils.SimpleEvent{Value: 2}, "bob")
		bobResult := <-bob
		require.NoError(t, bobResult.err)
		assert.Equal(t, testutils.SimpleEvent{Value: 2}, bobResult.reply)

		select {
		case res := <-alice:
			require.FailNow(t, "Bob's reply resolved Alice's request", "result: %+v", res)
		default:
		}

		tr.Publish(t.Context(), testutils.SimpleEvent{Value: 1}, "alice")
		aliceResult := <-alice
		require.NoError(t, aliceResult.err)
		assert.Equal(t, testutils.SimpleEvent{Value: 1}, aliceResult.reply)

		// Each request removed its own waiter before returning, leaving no empty map entries behind.
		tr.clients.mu.RLock()
		defer tr.clients.mu.RUnlock()
		assert.Empty(t, tr.clients.replyWaiters)
	})

	t.Run("broadcast resolves every pending request", func(t *testing.T) {
		t.Parallel()
		tr, address, waiting := newReplyRoutingTransport(t)
		alice := startReplyRequest(t, tr, address, waiting, "alice")
		bob := startReplyRequest(t, tr, address, waiting, "bob")

		tr.Publish(t.Context(), testutils.SimpleEvent{Value: 3}, "")
		for _, results := range []<-chan replyResult{alice, bob} {
			res := <-results
			require.NoError(t, res.err)
			assert.Equal(t, testutils.SimpleEvent{Value: 3}, res.reply)
		}
	})
}

// -------------------------------------------------------------------------------------------------
// SendCommandWithReply reply timeout
// -------------------------------------------------------------------------------------------------
// Options.ReplyTimeout caps the wait for a reply after dispatch. A request with no reply within the
// cap fails with DeadlineExceeded. The cap does not count the handler's own time, and zero leaves the
// wait to the client's deadline.
// -------------------------------------------------------------------------------------------------

// newReplyTimeoutTransport returns a transport with the given reply cap whose SimpleCommand handler
// signals waiting when it starts, sleeps for handlerDelay, then signals dispatched before returning.
func newReplyTimeoutTransport(
	t *testing.T, replyTimeout, handlerDelay time.Duration,
) (*Transport, *micro.ServiceAddress, <-chan struct{}, <-chan struct{}) {
	t.Helper()
	address := RandServiceAddress(testutils.NewRand(t))
	tr, err := New(Options{
		Address:      address,
		AuthMode:     AuthModeDev,
		Telemetry:    &telemetry.Telemetry{Logger: zerolog.Nop()},
		ReplyTimeout: replyTimeout,
	})
	require.NoError(t, err)
	started := make(chan struct{}, 1)
	done := make(chan struct{}, 1)
	tr.Handle(testutils.SimpleCommand{}.Name(), func(context.Context, *iscv1.Command, Sender) error {
		started <- struct{}{}
		time.Sleep(handlerDelay)
		done <- struct{}{}
		return nil
	})
	return tr, address, started, done
}

func TestTransport_SendCommandWithReply_ReplyTimeout(t *testing.T) {
	t.Parallel()

	t.Run("no reply within the cap fails with deadline exceeded", func(t *testing.T) {
		t.Parallel()
		tr, address, waiting, _ := newReplyTimeoutTransport(t, 50*time.Millisecond, 0)
		res := <-startReplyRequest(t, tr, address, waiting, "alice")
		require.Error(t, res.err)
		assert.Equal(t, connect.CodeDeadlineExceeded, connect.CodeOf(res.err))
	})

	t.Run("cap starts after dispatch", func(t *testing.T) {
		t.Parallel()
		// The handler takes twice the cap, so a cap that counted it would fail the request.
		const replyTimeout = 200 * time.Millisecond
		tr, address, waiting, dispatched := newReplyTimeoutTransport(t, replyTimeout, 2*replyTimeout)
		results := startReplyRequest(t, tr, address, waiting, "alice")

		<-dispatched
		tr.Publish(t.Context(), testutils.SimpleEvent{Value: 1}, "alice")
		res := <-results
		require.NoError(t, res.err)
		assert.Equal(t, testutils.SimpleEvent{Value: 1}, res.reply)
	})

	t.Run("zero cap waits until the client's deadline", func(t *testing.T) {
		t.Parallel()
		tr, address, _, _ := newReplyTimeoutTransport(t, 0, 0)
		ctx, cancel := context.WithTimeout(transportTestContext("alice"), 100*time.Millisecond)
		defer cancel()
		_, err := tr.clients.SendCommandWithReply(ctx, connect.NewRequest(&cardinalv1.SendCommandWithReplyRequest{
			Command: &iscv1.Command{
				Name:    testutils.SimpleCommand{}.Name(),
				Address: address,
				Payload: testutils.SimpleCommand{}.MarshalWire(),
			},
			EventName: testutils.SimpleEvent{}.Name(),
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeCanceled, connect.CodeOf(err))
	})
}

// -------------------------------------------------------------------------------------------------
// Inter-shard smoke tests
// -------------------------------------------------------------------------------------------------
// Verifies that a command enqueued and flushed by one transport is received over NATS by another,
// with this shard's address as its sender.
// -------------------------------------------------------------------------------------------------

func TestTransport_EnqueueFlush(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	fixtureA := newTransportFixture(t, prng, true)
	fixtureB := newTransportFixture(t, prng, true)

	payload := testutils.SimpleCommand{Value: prng.IntN(1_000_000)}
	fixtureA.tr.Enqueue(context.Background(), fixtureB.address, payload)
	fixtureA.tr.Flush()

	cmds := awaitCommands(t, fixtureB, 1)
	assert.Equal(t, payload, decodeSimpleCommand(t, cmds[0]))
	assert.Equal(t, ShardSender(fixtureA.address), fixtureB.received.lastSender())
}

// -------------------------------------------------------------------------------------------------
// Lifecycle
// -------------------------------------------------------------------------------------------------

func TestTransport_StopBeforeStart(t *testing.T) {
	t.Parallel()

	fixture := newTransportFixture(t, testutils.NewRand(t), false)
	fixture.tr.Flush() // No NATS connection yet: a no-op, as for a world that never started.
	require.NoError(t, fixture.tr.Stop(context.Background()))
}

func TestTransport_Handle(t *testing.T) {
	t.Parallel()

	t.Run("duplicate name panics", func(t *testing.T) {
		t.Parallel()
		fixture := newTransportFixture(t, testutils.NewRand(t), false)
		assert.Panics(t, func() {
			fixture.tr.Handle(testutils.SimpleCommand{}.Name(), fixture.received.handle)
		})
	})

	t.Run("after start panics", func(t *testing.T) {
		t.Parallel()
		fixture := newTransportFixture(t, testutils.NewRand(t), true)
		assert.Panics(t, func() {
			fixture.tr.Handle("late_command", fixture.received.handle)
		})
	})

	t.Run("after handler panics", func(t *testing.T) {
		t.Parallel()
		fixture := newTransportFixture(t, testutils.NewRand(t), false)
		_, _, err := fixture.tr.Handler()
		require.NoError(t, err)
		assert.Panics(t, func() {
			fixture.tr.Handle("late_command", fixture.received.handle)
		})
	})
}

func TestTransport_Register(t *testing.T) {
	t.Parallel()

	t.Run("client command arrives decoded with the player as sender", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		tr, address, got := newTypedTransport(t, prng)

		payload := testutils.SimpleCommand{Value: prng.IntN(1_000_000)}
		userID := testutils.RandString(prng, 8)
		_, err := tr.clients.SendCommand(
			transportTestContext(userID),
			connect.NewRequest(&cardinalv1.SendCommandRequest{
				Command: &iscv1.Command{
					Name:    payload.Name(),
					Address: address,
					Payload: payload.MarshalWire(),
				},
			}),
		)
		require.NoError(t, err)
		cmds := got.take()
		require.Len(t, cmds, 1)
		assert.Equal(t, Command[testutils.SimpleCommand]{Payload: payload, Sender: PlayerSender(userID)}, cmds[0])
	})

	t.Run("shard command arrives decoded with the sending shard as sender", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		tr, address, got := newTypedTransport(t, prng)
		require.NoError(t, tr.Start(NewTestClient(t)))
		t.Cleanup(func() { _ = tr.Stop(context.Background()) })
		sender := newTransportFixture(t, prng, true)

		payload := testutils.SimpleCommand{Value: prng.IntN(1_000_000)}
		sender.tr.Enqueue(context.Background(), address, payload)
		sender.tr.Flush()
		require.NoError(t, sender.tr.Stop(context.Background())) // Returns once the command is acked

		cmds := got.take()
		require.Len(t, cmds, 1)
		assert.Equal(t, Command[testutils.SimpleCommand]{Payload: payload, Sender: ShardSender(sender.address)},
			cmds[0])
	})

	t.Run("undecodable payload is rejected to the client", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		tr, address, got := newTypedTransport(t, prng)

		_, err := tr.clients.SendCommand(
			transportTestContext("user"),
			connect.NewRequest(&cardinalv1.SendCommandRequest{
				Command: &iscv1.Command{
					Name:    testutils.SimpleCommand{}.Name(),
					Address: address,
					Payload: []byte{1, 2, 3}, // Too short for the eight bytes SimpleCommand decodes
				},
			}),
		)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.Contains(t, err.Error(), "failed to decode command")
		assert.Empty(t, got.take())
	})

	t.Run("command name already handled panics", func(t *testing.T) {
		t.Parallel()
		fixture := newTransportFixture(t, testutils.NewRand(t), false)
		assert.Panics(t, func() {
			fixture.tr.RegisterCommand[testutils.SimpleCommand](nil)
		})
	})

	t.Run("after start panics", func(t *testing.T) {
		t.Parallel()
		fixture := newTransportFixture(t, testutils.NewRand(t), true)
		assert.Panics(t, func() { fixture.tr.RegisterCommand[testutils.SimpleCommand](nil) })
		assert.Panics(t, func() { fixture.tr.RegisterEvent[testutils.SimpleEvent]() })
	})

	t.Run("event before start", func(t *testing.T) {
		t.Parallel()
		fixture := newTransportFixture(t, testutils.NewRand(t), false)
		assert.NotPanics(t, func() { fixture.tr.RegisterEvent[testutils.SimpleEvent]() })
	})
}

// typedRecorder keeps every decoded command a RegisterCommand handler receives.
type typedRecorder struct {
	mu   sync.Mutex
	cmds []Command[testutils.SimpleCommand]
}

func (r *typedRecorder) handle(_ context.Context, cmd Command[testutils.SimpleCommand]) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cmds = append(r.cmds, cmd)
	return nil
}

func (r *typedRecorder) take() []Command[testutils.SimpleCommand] {
	r.mu.Lock()
	defer r.mu.Unlock()
	cmds := r.cmds
	r.cmds = nil
	return cmds
}

// newTypedTransport creates an unstarted transport that registers SimpleCommand through RegisterCommand,
// and returns it with its address and what its handler receives.
func newTypedTransport(t *testing.T, prng *rand.Rand) (*Transport, *micro.ServiceAddress, *typedRecorder) {
	t.Helper()
	address := RandServiceAddress(prng)
	tr, err := New(Options{
		Address:   address,
		AuthMode:  AuthModeDev,
		Telemetry: &telemetry.Telemetry{Logger: zerolog.Nop()},
	})
	require.NoError(t, err)
	got := &typedRecorder{}
	tr.RegisterCommand[testutils.SimpleCommand](got.handle)
	return tr, address, got
}

func TestTransport_StartTwice(t *testing.T) {
	t.Parallel()

	fixture := newTransportFixture(t, testutils.NewRand(t), true)
	service := fixture.tr.microService
	require.Error(t, fixture.tr.Start(NewTestClient(t)))
	assert.Same(t, service, fixture.tr.microService, "second Start replaced the first endpoints")
}

func TestTransport_StartWithoutClient(t *testing.T) {
	t.Parallel()

	fixture := newTransportFixture(t, testutils.NewRand(t), false)
	require.Error(t, fixture.tr.Start(nil))
}

func TestNew_InvalidOptions(t *testing.T) {
	t.Parallel()

	valid := func() Options {
		return Options{
			Address:   RandServiceAddress(testutils.NewRand(t)),
			AuthMode:  AuthModeDev,
			Telemetry: &telemetry.Telemetry{Logger: zerolog.Nop()},
		}
	}
	tests := map[string]func(*Options){
		"no address":             func(o *Options) { o.Address = nil },
		"undefined auth mode":    func(o *Options) { o.AuthMode = AuthModeUndefined },
		"argus without url":      func(o *Options) { o.AuthMode = AuthModeArgus },
		"no telemetry":           func(o *Options) { o.Telemetry = nil },
		"negative reply timeout": func(o *Options) { o.ReplyTimeout = -time.Second },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			opts := valid()
			mutate(&opts)
			_, err := New(opts)
			require.Error(t, err)
		})
	}
}

// -------------------------------------------------------------------------------------------------
// Fixture
// -------------------------------------------------------------------------------------------------

// recorder is a Handler that keeps every command it receives, and who sent it.
type recorder struct {
	mu      sync.Mutex
	cmds    []*iscv1.Command
	senders []Sender
	last    Sender // Sender of the latest command, kept across takes
}

func (r *recorder) handle(_ context.Context, cmd *iscv1.Command, sender Sender) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cmds = append(r.cmds, cmd)
	r.senders = append(r.senders, sender)
	r.last = sender
	return nil
}

// take returns the commands received since the last take.
func (r *recorder) take() []*iscv1.Command {
	cmds, _ := r.takeWithSenders()
	return cmds
}

// takeWithSenders returns the commands received since the last take, and their senders.
func (r *recorder) takeWithSenders() ([]*iscv1.Command, []Sender) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cmds, senders := r.cmds, r.senders
	r.cmds, r.senders = nil, nil
	return cmds, senders
}

// lastSender returns the sender of the latest command received.
func (r *recorder) lastSender() Sender {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last
}

type transportFixture struct {
	address  *micro.ServiceAddress
	tr       *Transport
	received *recorder
}

// newTransportFixture creates a transport that handles SimpleCommand. With start, it is started with its
// own client on the test NATS server, and stopped before that client closes when the test ends.
func newTransportFixture(t *testing.T, prng *rand.Rand, start bool) *transportFixture {
	t.Helper()

	address := RandServiceAddress(prng)
	tr, err := New(Options{
		Address:   address,
		AuthMode:  AuthModeDev,
		Telemetry: &telemetry.Telemetry{Logger: zerolog.Nop()},
	})
	require.NoError(t, err)

	received := &recorder{}
	tr.Handle(testutils.SimpleCommand{}.Name(), received.handle)

	if start {
		require.NoError(t, tr.Start(NewTestClient(t)))
		t.Cleanup(func() { _ = tr.Stop(context.Background()) })
	}

	return &transportFixture{address: address, tr: tr, received: received}
}

// awaitCommands waits until fixture has received n commands, and returns them.
func awaitCommands(t *testing.T, fixture *transportFixture, n int) []*iscv1.Command {
	t.Helper()
	var cmds []*iscv1.Command
	require.Eventually(t, func() bool {
		cmds = append(cmds, fixture.received.take()...)
		return len(cmds) >= n
	}, 5*time.Second, 10*time.Millisecond)
	require.Len(t, cmds, n)
	return cmds
}

func decodeSimpleCommand(t *testing.T, cmd *iscv1.Command) testutils.SimpleCommand {
	t.Helper()
	decoded, err := testutils.SimpleCommand{}.UnmarshalWire(cmd.GetPayload())
	require.NoError(t, err)
	payload, ok := decoded.(testutils.SimpleCommand)
	require.True(t, ok)
	return payload
}

func transportTestContext(userID string) context.Context {
	return authn.SetInfo(context.Background(), &Player{ID: userID})
}
