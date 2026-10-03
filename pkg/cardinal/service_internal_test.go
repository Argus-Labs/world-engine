package cardinal

import (
	"context"
	"math/rand/v2"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/introspect"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/argus-labs/world-engine/pkg/transport"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// -------------------------------------------------------------------------------------------------
// Inter-shard wiring smoke tests
// -------------------------------------------------------------------------------------------------
// Verifies that an inter-shard command dispatched by one world is sent by its transport, and that the
// target world's transport hands it to the command manager with the sending shard as persona.
// -------------------------------------------------------------------------------------------------

func TestService_SendInterShardCommand(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	fixtureA := newServiceFixture(t, prng, true)
	fixtureB := newServiceFixture(t, prng, true)

	payload := testutils.SimpleCommand{Value: prng.IntN(1_000_000)}
	require.NoError(t, fixtureA.world.sendInterShardCommand(context.Background(), event.Event{
		Kind: event.KindInterShardCommand,
		Payload: command.Command{
			Name:    payload.Name(),
			Address: fixtureB.world.address,
			Payload: payload,
		},
	}))
	fixtureA.world.transport.Flush() // what the tick does after dispatch

	cmds := awaitCommands(t, fixtureB)
	assert.Equal(t, payload, cmds[0].Payload)
	assert.Equal(t, micro.String(fixtureA.world.address), cmds[0].Persona)
}

func TestService_RegisterCommandTwice(t *testing.T) {
	t.Parallel()

	fixture := newServiceFixture(t, testutils.NewRand(t), false)
	require.NotPanics(t, func() { fixture.world.RegisterCommand[testutils.SimpleCommand]() })
}

func TestService_DebugServiceFinalizesIntrospection(t *testing.T) {
	t.Parallel()

	fixture := newServiceFixture(t, testutils.NewRand(t), false)
	debug := newIntrospectionTestModule()
	require.NoError(t, debug.register(introspect.Command, introspectionSample{}))
	fixture.world.debug = debug
	fixture.world.options.NATSConfig = &micro.NATSConfig{Name: "test-service", URL: TestNATS.ClientURL()}

	// Through startTransport and the world's own server, so the test fails if startup stops finalizing
	// the catalog or stops mounting the debug service.
	require.NoError(t, fixture.world.startTransport("127.0.0.1:0"))
	t.Cleanup(func() {
		_ = fixture.world.stopTransport(context.Background())
		fixture.world.client.Close()
	})

	client := cardinalv1connect.NewDebugServiceClient(http.DefaultClient, "http://"+fixture.world.server.Addr)
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

// -------------------------------------------------------------------------------------------------
// Fixture
// -------------------------------------------------------------------------------------------------

// awaitCommands drains fixture's world until its SimpleCommand queue has one command, and returns it.
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
	world     *World
	commandID command.ID
}

// newServiceFixture creates a world wired to a transport, with SimpleCommand registered. With start, the
// transport is started with its own client on the test NATS server, and stopped when the test ends.
func newServiceFixture(t *testing.T, prng *rand.Rand, start bool) *serviceFixture {
	t.Helper()

	w := &World{
		world:    ecs.NewWorld(),
		commands: command.NewManager(),
		events:   event.NewManager(1024),
		address:  RandServiceAddress(prng),
		tel:      telemetry.Telemetry{Logger: zerolog.Nop()},
	}
	w.transport = newTestTransport(t, w)
	w.events.RegisterHandler(event.KindDefault, w.publishEvent)
	w.events.RegisterHandler(event.KindInterShardCommand, w.sendInterShardCommand)

	w.RegisterCommand[testutils.SimpleCommand]()
	cmdID, ok := w.commands.Lookup(testutils.SimpleCommand{}.Name())
	require.True(t, ok)

	if start {
		require.NoError(t, w.transport.Start(newTestClient(t)))
		t.Cleanup(func() { _ = w.transport.Stop(context.Background()) })
	}

	return &serviceFixture{world: w, commandID: cmdID}
}

// newTestTransport creates an unstarted transport for w.
func newTestTransport(t *testing.T, w *World) *transport.Transport {
	t.Helper()

	tr, err := transport.New(transport.Options{
		Address:   w.address,
		AuthMode:  AuthModeDev,
		Telemetry: &w.tel,
	})
	require.NoError(t, err)
	return tr
}

// newTestClient connects to the test NATS server, and closes the connection when the test ends.
func newTestClient(t *testing.T) *micro.Client {
	t.Helper()

	client, err := micro.NewClient(
		micro.WithNATSConfig(micro.NATSConfig{Name: "test-service", URL: TestNATS.ClientURL()}),
		micro.WithLogger(zerolog.Nop()),
	)
	require.NoError(t, err)
	t.Cleanup(client.Close)
	return client
}
