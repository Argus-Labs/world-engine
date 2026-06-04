package cardinal

import (
	"context"
	"math/rand/v2"
	"testing"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

// -------------------------------------------------------------------------------------------------
// SendCommand smoke tests
// -------------------------------------------------------------------------------------------------
// Verifies that the ConnectRPC command handler enqueues commands into the command manager and
// rejects commands addressed to the wrong shard.
// -------------------------------------------------------------------------------------------------

func TestService_SendCommand(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newServiceFixture(t, prng, false)

		payload := testutils.SimpleCommand{Value: prng.IntN(1_000_000)}
		payloadBytes, err := schema.Serialize(payload)
		require.NoError(t, err)
		userID := testutils.RandString(prng, 8)
		cmdPb := &iscv1.Command{
			Name:    payload.Name(),
			Address: fixture.world.address,
			Persona: &iscv1.Persona{Id: "client-provided-persona"},
			Payload: payloadBytes,
		}

		_, err = fixture.svc.SendCommand(
			serviceTestContext(userID),
			connect.NewRequest(&cardinalv1.SendCommandRequest{Command: cmdPb}),
		)
		require.NoError(t, err)

		fixture.world.commands.Drain()
		cmds, err := fixture.world.commands.Get(fixture.commandID)
		require.NoError(t, err)
		require.Len(t, cmds, 1)
		assert.Equal(t, payload, cmds[0].Payload)
		assert.Equal(t, userID, cmds[0].Persona)
	})

	t.Run("wrong address rejected", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newServiceFixture(t, prng, false)

		payload := testutils.SimpleCommand{Value: 42}
		payloadBytes, err := schema.Serialize(payload)
		require.NoError(t, err)
		cmdPb := &iscv1.Command{
			Name:    payload.Name(),
			Address: RandServiceAddress(prng),
			Persona: &iscv1.Persona{Id: "client-provided-persona"},
			Payload: payloadBytes,
		}

		_, err = fixture.svc.SendCommand(
			serviceTestContext(testutils.RandString(prng, 8)),
			connect.NewRequest(&cardinalv1.SendCommandRequest{Command: cmdPb}),
		)
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.Contains(t, err.Error(), "address")
	})
}

// -------------------------------------------------------------------------------------------------
// Query smoke tests
// -------------------------------------------------------------------------------------------------
// Verifies that the ConnectRPC query handler accepts a query proto, executes it against the world
// state, and returns a well-formed response.
// -------------------------------------------------------------------------------------------------

func TestService_Query(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newServiceFixture(t, prng, false)

		resp, err := fixture.svc.Query(
			context.Background(),
			connect.NewRequest(&cardinalv1.QueryRequest{
				Address: fixture.world.address,
				Query: &iscv1.Query{
					Match: iscv1.Query_MATCH_ALL,
				},
			}),
		)
		require.NoError(t, err)
		require.NotNil(t, resp.Msg.GetResults())
		assert.Empty(t, resp.Msg.GetResults().GetEntities())
	})
}

// -------------------------------------------------------------------------------------------------
// publishDefaultEvent smoke tests
// -------------------------------------------------------------------------------------------------
// Verifies that publishing a default event serializes the payload and delivers it to registered
// ConnectRPC reply waiters with round-trip integrity.
// -------------------------------------------------------------------------------------------------

func TestService_PublishDefaultEvent(t *testing.T) {
	t.Parallel()

	t.Run("reply waiter", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newServiceFixture(t, prng, false)

		payload := testutils.SimpleEvent{Value: prng.Int()}
		waiter := fixture.svc.addReplyWaiter(payload.Name())
		defer fixture.svc.removeReplyWaiter(payload.Name(), waiter)

		err := fixture.svc.publishDefaultEvent(event.Event{
			Kind:    event.KindDefault,
			Payload: payload,
		})
		require.NoError(t, err)

		eventPb := <-waiter
		assert.Equal(t, payload.Name(), eventPb.GetName())
		var decoded testutils.SimpleEvent
		require.NoError(t, schema.Deserialize(eventPb.GetPayload(), &decoded))
		assert.Equal(t, payload, decoded)
	})
}

// -------------------------------------------------------------------------------------------------
// publishInterShardCommand smoke tests
// -------------------------------------------------------------------------------------------------
// Verifies that an inter-shard command published by one service is received over the NATS ISC path
// and enqueued by the target service with correct payload and sender shard.
// -------------------------------------------------------------------------------------------------

func TestService_PublishInterShardCommand(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)

		// Stand up two services on the same NATS.
		fixtureA := newServiceFixture(t, prng, true)
		fixtureB := newServiceFixture(t, prng, true)

		// Have service A send an inter-shard command targeting service B.
		payload := testutils.SimpleCommand{Value: prng.IntN(1_000_000)}
		sender := micro.String(fixtureA.world.address)
		err := fixtureA.svc.publishInterShardCommand(event.Event{
			Kind: event.KindInterShardCommand,
			Payload: command.Command{
				Name:    payload.Name(),
				Address: fixtureB.world.address,
				Persona: sender,
				Payload: payload,
			},
		})
		require.NoError(t, err)

		// Drain service B and verify the command arrived with correct payload/persona.
		fixtureB.world.commands.Drain()
		cmds, err := fixtureB.world.commands.Get(fixtureB.commandID)
		require.NoError(t, err)
		require.Len(t, cmds, 1)
		assert.Equal(t, payload, cmds[0].Payload)
		assert.Equal(t, sender, cmds[0].Persona)
	})
}

// -------------------------------------------------------------------------------------------------
// Fixture
// -------------------------------------------------------------------------------------------------

type serviceFixture struct {
	client    *micro.Client
	svc       *service
	world     *World
	commandID command.ID
}

func newServiceFixture(t *testing.T, prng *rand.Rand, registerNATSEndpoints bool) *serviceFixture {
	t.Helper()

	address := RandServiceAddress(prng)
	tel := telemetry.Telemetry{
		Logger: zerolog.Nop(),
		Tracer: noop.NewTracerProvider().Tracer("test"),
	}

	w := &World{
		world:    ecs.NewWorld(),
		commands: command.NewManager(),
		events:   event.NewManager(1024),
		address:  address,
		tel:      tel,
	}

	queue := command.NewQueue[testutils.SimpleCommand]()
	cmdID, err := w.commands.Register(testutils.SimpleCommand{}.Name(), queue)
	require.NoError(t, err)

	svc := newService(w, AuthModePassthrough, "")
	svc.registerCommandHandler(testutils.SimpleCommand{}.Name())
	w.service = svc

	fixture := &serviceFixture{
		svc:       svc,
		world:     w,
		commandID: cmdID,
	}

	if registerNATSEndpoints {
		client := NewTestClient(t)
		svc.client = client
		fixture.client = client

		microService, err := micro.NewService(client, address, &tel)
		require.NoError(t, err)
		t.Cleanup(func() { _ = microService.Close() })
		svc.microService = microService

		require.NoError(t, microService.AddEndpoint("ping", svc.handlePing))
		require.NoError(t, microService.AddGroup("command").AddEndpoint(
			testutils.SimpleCommand{}.Name(),
			svc.handleInterShardCommand,
		))
		require.NoError(t, client.Flush())
	}

	return fixture
}

func serviceTestContext(userID string) context.Context {
	return authn.SetInfo(context.Background(), &User{ID: userID})
}
