package cardinal

import (
	"context"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/introspect"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
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
		payloadBytes := payload.MarshalWire()
		userID := testutils.RandString(prng, 8)
		cmdPb := &iscv1.Command{
			Name:    payload.Name(),
			Address: fixture.world.address,
			Persona: &iscv1.Persona{Id: "client-provided-persona"},
			Payload: payloadBytes,
		}

		_, err := fixture.svc.SendCommand(
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
		payloadBytes := payload.MarshalWire()
		cmdPb := &iscv1.Command{
			Name:    payload.Name(),
			Address: RandServiceAddress(prng),
			Persona: &iscv1.Persona{Id: "client-provided-persona"},
			Payload: payloadBytes,
		}

		_, err := fixture.svc.SendCommand(
			serviceTestContext(testutils.RandString(prng, 8)),
			connect.NewRequest(&cardinalv1.SendCommandRequest{Command: cmdPb}),
		)
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.Contains(t, err.Error(), "address")
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

		err := fixture.svc.publishDefaultEvent(context.Background(), event.Event{
			Kind:    event.KindDefault,
			Payload: payload,
		})
		require.NoError(t, err)

		eventPb := <-waiter
		assert.Equal(t, payload.Name(), eventPb.GetName())
		decoded, err := testutils.SimpleEvent{}.UnmarshalWire(eventPb.GetPayload())
		require.NoError(t, err)
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
		err := fixtureA.svc.publishInterShardCommand(context.Background(), event.Event{
			Kind: event.KindInterShardCommand,
			Payload: command.Command{
				Name:    payload.Name(),
				Address: fixtureB.world.address,
				Persona: sender,
				Payload: payload,
			},
		})
		require.NoError(t, err)
		fixtureA.svc.drainInterShardCommands() // what the tick does after dispatch

		// The send is asynchronous: drain service B until the command arrives, then verify its
		// payload/persona.
		cmds := awaitCommands(t, fixtureB)
		assert.Equal(t, payload, cmds[0].Payload)
		assert.Equal(t, sender, cmds[0].Persona)
	})
}

func TestService_MountDebugServiceFinalizesIntrospection(t *testing.T) {
	t.Parallel()

	fixture := newServiceFixture(t, testutils.NewRand(t), false)
	debug := newIntrospectionTestModule()
	require.NoError(t, debug.register(introspect.Command, introspectionSample{}))
	fixture.world.debug = debug

	mux := http.NewServeMux()
	require.NoError(t, fixture.svc.mountDebugService(mux))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client := cardinalv1connect.NewDebugServiceClient(server.Client(), server.URL)
	response, err := client.Introspect(
		context.Background(),
		connect.NewRequest(&cardinalv1.IntrospectRequest{}),
	)
	require.NoError(t, err)
	require.NotEmpty(t, response.Msg.GetProtoDescriptorSet())
	require.NotEmpty(t, response.Msg.GetCommands())

	var set descriptorpb.FileDescriptorSet
	require.NoError(t, proto.Unmarshal(response.Msg.GetProtoDescriptorSet(), &set))
	files, err := protodesc.NewFiles(&set)
	require.NoError(t, err)
	for _, command := range response.Msg.GetCommands() {
		_, err := files.FindDescriptorByName(protoreflect.FullName(command.GetProtoMessageName()))
		require.NoError(t, err)
	}
}

func TestService_ShutdownBeforeInitializationCompletes(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&service{}).shutdown(context.Background()))
}

// -------------------------------------------------------------------------------------------------
// Fixture
// -------------------------------------------------------------------------------------------------

// awaitCommands drains fixture's world until its SimpleCommand queue has one command, which it
// returns. Inter-shard sends are asynchronous, so the command lands some time after the publish.
func awaitCommands(t *testing.T, fixture *serviceFixture) []command.Command {
	t.Helper()
	var cmds []command.Command
	require.Eventually(t, func() bool {
		fixture.world.commands.Drain()
		var err error
		cmds, err = fixture.world.commands.Get(fixture.commandID)
		require.NoError(t, err)
		return len(cmds) == 1
	}, 5*time.Second, 10*time.Millisecond)
	return cmds
}

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
	}

	w := &World{
		world:    ecs.NewWorld(),
		commands: command.NewManager(),
		events:   event.NewManager(1024),
		address:  address,
		tel:      tel,
	}

	svc := newService(w, AuthModeDev, "")
	w.service = svc

	// RegisterCommand is what makes the service accept SimpleCommand from clients.
	w.RegisterCommand[testutils.SimpleCommand]()
	cmdID, ok := w.commands.Lookup(testutils.SimpleCommand{}.Name())
	require.True(t, ok)

	fixture := &serviceFixture{
		svc:       svc,
		world:     w,
		commandID: cmdID,
	}

	if registerNATSEndpoints {
		client := NewTestClient(t)
		svc.client = client
		fixture.client = client
		svc.interShard = newInterShard(address, client, &w.commands, zerolog.Nop())
		// Registered after the client, so it runs first: queued sends finish before the client closes.
		t.Cleanup(func() { svc.interShard.stop(context.Background()) })

		microService, err := micro.NewService(client, address, &tel)
		require.NoError(t, err)
		t.Cleanup(func() { _ = microService.Close() })
		svc.microService = microService

		require.NoError(t, microService.AddEndpoint("ping", svc.handlePing))
		require.NoError(t, svc.interShard.start(microService, svc.commands))
		require.NoError(t, client.Flush())
	}

	return fixture
}

func serviceTestContext(userID string) context.Context {
	return authn.SetInfo(context.Background(), &User{ID: userID})
}
