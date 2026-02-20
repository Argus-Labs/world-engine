package cardinal

import (
	"context"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/argus-labs/world-engine/pkg/testutils"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// -------------------------------------------------------------------------------------------------
// handleCommand smoke tests
// -------------------------------------------------------------------------------------------------
// Verifies that the NATS command handler correctly deserializes incoming commands, enqueues them
// into the command manager, and rejects commands addressed to the wrong shard.
// -------------------------------------------------------------------------------------------------

func TestService_HandleCommand(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newServiceFixture(t, prng)

		// Send a valid command over NATS.
		payload := testutils.SimpleCommand{Value: prng.IntN(1_000_000)}
		persona := testutils.RandString(prng, 8)
		payloadBytes, err := schema.Serialize(payload)
		require.NoError(t, err)
		cmdPb := &iscv1.Command{
			Name:    payload.Name(),
			Address: fixture.world.address,
			Persona: &iscv1.Persona{Id: persona},
			Payload: payloadBytes,
		}
		endpoint := micro.Endpoint(fixture.world.address, "command."+cmdPb.GetName())
		resp := fixture.rawRequest(t, endpoint, cmdPb)
		require.Equal(t, int32(codes.OK), resp.GetStatus().GetCode())

		// Drain into read buffers and verify round-trip integrity.
		fixture.world.commands.Drain()
		cmds, err := fixture.world.commands.Get(fixture.commandID)
		require.NoError(t, err)
		require.Len(t, cmds, 1)
		assert.Equal(t, payload, cmds[0].Payload)
		assert.Equal(t, persona, cmds[0].Persona)
	})

	t.Run("wrong address rejected", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newServiceFixture(t, prng)

		// Build a command with a different shard address than the fixture's.
		payload := testutils.SimpleCommand{Value: 42}
		payloadBytes, err := schema.Serialize(payload)
		require.NoError(t, err)
		wrongAddress := randServiceAddress(prng)
		cmdPb := &iscv1.Command{
			Name:    payload.Name(),
			Address: wrongAddress,
			Persona: &iscv1.Persona{Id: "player"},
			Payload: payloadBytes,
		}

		// Send and verify it is rejected with InvalidArgument.
		endpoint := micro.Endpoint(fixture.world.address, "command."+cmdPb.GetName())
		resp := fixture.rawRequest(t, endpoint, cmdPb)
		assert.Equal(t, int32(codes.InvalidArgument), resp.GetStatus().GetCode())
		assert.Contains(t, resp.GetStatus().GetMessage(), "address")
	})
}

// -------------------------------------------------------------------------------------------------
// handleQuery smoke tests
// -------------------------------------------------------------------------------------------------
// Verifies that the NATS query handler accepts a query proto, executes it against the world state,
// and returns a well-formed response.
// -------------------------------------------------------------------------------------------------

func TestService_HandleQuery(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newServiceFixture(t, prng)

		// Send a MATCH_ALL query to the empty world.
		queryPb := &iscv1.Query{
			Match: iscv1.Query_MATCH_ALL,
		}
		endpoint := micro.Endpoint(fixture.world.address, "query")
		resp := fixture.rawRequest(t, endpoint, queryPb)
		require.Equal(t, int32(codes.OK), resp.GetStatus().GetCode())

		// Verify response is a valid QueryResult with no entities.
		var result iscv1.QueryResult
		require.NoError(t, resp.GetPayload().UnmarshalTo(&result))
		assert.Empty(t, result.GetEntities())
	})
}

// -------------------------------------------------------------------------------------------------
// publishDefaultEvent smoke tests
// -------------------------------------------------------------------------------------------------
// Verifies that publishing a default event serializes the payload and delivers it to the correct
// NATS subject so that subscribers receive the event with round-trip integrity.
// -------------------------------------------------------------------------------------------------

func TestService_PublishDefaultEvent(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newServiceFixture(t, prng)

		payload := testutils.SimpleEvent{Value: prng.Int()}

		// Subscribe to the expected NATS subject before publishing.
		subject := micro.String(fixture.world.address) + ".event." + payload.Name()
		sub, err := fixture.client.SubscribeSync(subject)
		require.NoError(t, err)
		defer sub.Unsubscribe()
		require.NoError(t, fixture.client.Flush())

		// Publish the event.
		err = fixture.svc.publishDefaultEvent(event.Event{
			Kind:    event.KindDefault,
			Payload: payload,
		})
		require.NoError(t, err)
		require.NoError(t, fixture.client.Flush())

		// Receive and decode the NATS message.
		msg, err := sub.NextMsg(2 * time.Second)
		require.NoError(t, err)
		var eventPb iscv1.Event
		require.NoError(t, proto.Unmarshal(msg.Data, &eventPb))

		// Verify event name and payload round-trip.
		assert.Equal(t, payload.Name(), eventPb.GetName())
		var decoded testutils.SimpleEvent
		require.NoError(t, schema.Deserialize(eventPb.GetPayload(), &decoded))
		assert.Equal(t, payload, decoded)
	})
}

// -------------------------------------------------------------------------------------------------
// publishInterShardCommand smoke tests
// -------------------------------------------------------------------------------------------------
// Verifies that an inter-shard command published by one service is received and enqueued by the
// target service on the same NATS bus with correct payload and persona.
// -------------------------------------------------------------------------------------------------

func TestService_PublishInterShardCommand(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)

		// Stand up two services on the same NATS.
		fixtureA := newServiceFixture(t, prng)
		fixtureB := newServiceFixture(t, prng)
		require.NoError(t, fixtureA.client.Flush())
		require.NoError(t, fixtureB.client.Flush())

		// Have service A send an inter-shard command targeting service B.
		payload := testutils.SimpleCommand{Value: prng.IntN(1_000_000)}
		persona := testutils.RandString(prng, 8)
		err := fixtureA.svc.publishInterShardCommand(event.Event{
			Kind: event.KindInterShardCommand,
			Payload: command.Command{
				Name:    payload.Name(),
				Address: fixtureB.world.address,
				Persona: persona,
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
		assert.Equal(t, persona, cmds[0].Persona)
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

func newServiceFixture(t *testing.T, prng *rand.Rand) *serviceFixture {
	t.Helper()

	address := randServiceAddress(prng)
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

	// Register the command type so Enqueue works.
	queue := command.NewQueue[testutils.SimpleCommand]()
	cmdID, err := w.commands.Register(testutils.SimpleCommand{}.Name(), queue)
	require.NoError(t, err)

	svc := newService(w)
	svc.registerCommandHandler(testutils.SimpleCommand{}.Name())
	w.service = svc

	// Create a micro.Client connected to the shared test NATS server.
	client := newTestClient(t)
	svc.client = client

	microService, err := micro.NewService(client, address, &tel)
	require.NoError(t, err)
	t.Cleanup(func() { _ = microService.Close() })

	svc.service = microService

	// Register endpoints.
	require.NoError(t, microService.AddEndpoint("ping", svc.handlePing))
	require.NoError(t, microService.AddEndpoint("query", svc.handleQuery))
	require.NoError(t, microService.AddGroup("command").AddEndpoint(testutils.SimpleCommand{}.Name(), svc.handleCommand))

	// Flush to ensure all subscriptions are active before tests send requests.
	require.NoError(t, client.Flush())

	return &serviceFixture{
		client:    client,
		svc:       svc,
		world:     w,
		commandID: cmdID,
	}
}

// rawRequest wraps a proto.Message payload in a microv1.Request, sends it over NATS, and
// returns the parsed microv1.Response. This bypasses micro.Client.Request so we can inspect
// non-OK status codes without the client returning an error.
func (f *serviceFixture) rawRequest(t *testing.T, subject string, payload proto.Message) *microv1.Response {
	t.Helper()

	anyPayload, err := anypb.New(payload)
	require.NoError(t, err)

	reqPb := &microv1.Request{
		ServiceAddress: f.world.address,
		Payload:        anyPayload,
	}

	reqBytes, err := proto.Marshal(reqPb)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	msg, err := f.client.RequestWithContext(ctx, subject, reqBytes)
	require.NoError(t, err)

	var resp microv1.Response
	require.NoError(t, proto.Unmarshal(msg.Data, &resp))
	return &resp
}
