package transport

import (
	"context"
	"math/rand/v2"
	"net/http"
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
// Verifies that the ConnectRPC command handler stamps the persona, hands commands to the registered
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
			Persona: &iscv1.Persona{Id: "client-provided-persona"},
			Payload: payload.MarshalWire(),
		}

		_, err := fixture.tr.clients.SendCommand(
			transportTestContext(userID),
			connect.NewRequest(&cardinalv1.SendCommandRequest{Command: cmdPb}),
		)
		require.NoError(t, err)

		cmds := fixture.received.take()
		require.Len(t, cmds, 1)
		assert.Equal(t, payload, decodeSimpleCommand(t, cmds[0]))
		assert.Equal(t, userID, cmds[0].GetPersona().GetId())
	})

	t.Run("wrong address rejected", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newTransportFixture(t, prng, false)

		payload := testutils.SimpleCommand{Value: 42}
		cmdPb := &iscv1.Command{
			Name:    payload.Name(),
			Address: RandServiceAddress(prng),
			Persona: &iscv1.Persona{Id: "client-provided-persona"},
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
			Persona: &iscv1.Persona{Id: "client-provided-persona"},
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
		waiter := fixture.tr.clients.addReplyWaiter(payload.Name())
		defer fixture.tr.clients.removeReplyWaiter(payload.Name(), waiter)

		fixture.tr.Publish(context.Background(), payload, "")

		eventPb := <-waiter
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
		tr.Handle(testutils.SimpleCommand{}.Name(), func(ctx context.Context, _ *iscv1.Command) error {
			tr.Publish(ctx, reply, "")
			return nil
		})

		ctx, cancel := context.WithTimeout(transportTestContext(testutils.RandString(prng, 8)), 2*time.Second)
		defer cancel()
		res, err := tr.clients.SendCommandWithReply(ctx, connect.NewRequest(&cardinalv1.SendCommandWithReplyRequest{
			Command: &iscv1.Command{
				Name:    testutils.SimpleCommand{}.Name(),
				Address: address,
				Persona: &iscv1.Persona{Id: "client-provided-persona"},
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
// Inter-shard smoke tests
// -------------------------------------------------------------------------------------------------
// Verifies that a command enqueued and flushed by one transport is received over NATS by another,
// with this shard's address as its persona.
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
	assert.Equal(t, micro.String(fixtureA.address), cmds[0].GetPersona().GetId())
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
}

func TestTransport_StartTwice(t *testing.T) {
	t.Parallel()

	fixture := newTransportFixture(t, testutils.NewRand(t), true)
	client := fixture.tr.client
	require.Error(t, fixture.tr.Start("127.0.0.1:0"))
	assert.Same(t, client, fixture.tr.client, "second Start replaced the first connection")
}

func TestTransport_StartMountsServices(t *testing.T) {
	t.Parallel()

	var got []connect.HandlerOption
	tr, err := New(Options{
		Address:   RandServiceAddress(testutils.NewRand(t)),
		AuthMode:  AuthModeDev,
		NATS:      &micro.NATSConfig{Name: "test-transport", URL: TestNATS.ClientURL()},
		Telemetry: &telemetry.Telemetry{Logger: zerolog.Nop()},
		Services: []ServiceHandler{func(opts ...connect.HandlerOption) (string, http.Handler) {
			got = opts
			return "/extra.v1.ExtraService/", http.NotFoundHandler()
		}},
	})
	require.NoError(t, err)
	require.NoError(t, tr.Start("127.0.0.1:0"))
	t.Cleanup(func() { _ = tr.Stop(context.Background()) })

	assert.Len(t, got, 1, "extra services get CardinalService's interceptors")
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
		"no address":          func(o *Options) { o.Address = nil },
		"undefined auth mode": func(o *Options) { o.AuthMode = AuthModeUndefined },
		"argus without url":   func(o *Options) { o.AuthMode = AuthModeArgus },
		"no telemetry":        func(o *Options) { o.Telemetry = nil },
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

// recorder is a Handler that keeps every command it receives.
type recorder struct {
	mu   sync.Mutex
	cmds []*iscv1.Command
}

func (r *recorder) handle(_ context.Context, cmd *iscv1.Command) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cmds = append(r.cmds, cmd)
	return nil
}

// take returns the commands received since the last take.
func (r *recorder) take() []*iscv1.Command {
	r.mu.Lock()
	defer r.mu.Unlock()
	cmds := r.cmds
	r.cmds = nil
	return cmds
}

type transportFixture struct {
	address  *micro.ServiceAddress
	tr       *Transport
	received *recorder
}

// newTransportFixture creates a transport that handles SimpleCommand. With start, it is started on the
// test NATS server and a random local port, and stopped when the test ends.
func newTransportFixture(t *testing.T, prng *rand.Rand, start bool) *transportFixture {
	t.Helper()

	address := RandServiceAddress(prng)
	tr, err := New(Options{
		Address:   address,
		AuthMode:  AuthModeDev,
		NATS:      &micro.NATSConfig{Name: "test-transport", URL: TestNATS.ClientURL()},
		Telemetry: &telemetry.Telemetry{Logger: zerolog.Nop()},
	})
	require.NoError(t, err)

	received := &recorder{}
	tr.Handle(testutils.SimpleCommand{}.Name(), received.handle)

	if start {
		require.NoError(t, tr.Start("127.0.0.1:0"))
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
	return authn.SetInfo(context.Background(), &User{ID: userID})
}
