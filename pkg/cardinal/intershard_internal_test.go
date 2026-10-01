package cardinal

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/argus-labs/world-engine/pkg/testutils"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func simpleCommandTo(from, to *micro.ServiceAddress, value int) *iscv1.Command {
	payload := testutils.SimpleCommand{Value: value}
	return &iscv1.Command{
		Name:    payload.Name(),
		Address: to,
		Persona: &iscv1.Persona{Id: micro.String(from)},
		Payload: payload.MarshalWire(),
	}
}

// TestInterShard_StopSendsDrainedInOrder checks order across ticks, and that stop waits for every send.
func TestInterShard_StopSendsDrainedInOrder(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	fixtureA := newServiceFixture(t, prng, true)
	fixtureB := newServiceFixture(t, prng, true)
	link := newInterShard(fixtureA.world.address, fixtureA.client, &fixtureA.world.commands, zerolog.Nop())

	const ticks, perTick = 5, 4
	for tick := range ticks {
		for i := range perTick {
			link.enqueue(context.Background(),
				simpleCommandTo(fixtureA.world.address, fixtureB.world.address, tick*perTick+i))
		}
		link.drain()
	}
	link.stop(context.Background())

	// stop returned only after B accepted every send, so one drain sees them all.
	fixtureB.world.commands.Drain()
	cmds, err := fixtureB.world.commands.Get(fixtureB.commandID)
	require.NoError(t, err)
	require.Len(t, cmds, ticks*perTick)
	for i, cmd := range cmds {
		assert.Equal(t, testutils.SimpleCommand{Value: i}, cmd.Payload)
	}
}

// TestInterShard_HungTargetDoesNotBlockDrain checks that a target that never acks does not block drain,
// and that the commands behind it are sent in order once it recovers.
func TestInterShard_HungTargetDoesNotBlockDrain(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	fixtureA := newServiceFixture(t, prng, true)
	fixtureB := newServiceFixture(t, prng, true)

	hungAddress := RandServiceAddress(prng)
	release := make(chan struct{})
	var releaseOnce sync.Once
	unhang := func() { releaseOnce.Do(func() { close(release) }) }
	hungClient := NewTestClient(t)
	hung, err := micro.NewService(hungClient, hungAddress, &telemetry.Telemetry{Logger: zerolog.Nop()})
	require.NoError(t, err)
	require.NoError(t, hung.AddGroup("command").AddEndpoint(testutils.SimpleCommand{}.Name(),
		func(_ context.Context, req *micro.Request) *micro.Response {
			<-release
			return micro.NewSuccessResponse(req, nil)
		}))
	require.NoError(t, hungClient.Flush())

	link := newInterShard(fixtureA.world.address, fixtureA.client, &fixtureA.world.commands, zerolog.Nop())
	t.Cleanup(func() {
		unhang()
		link.stop(context.Background())
		_ = hung.Close()
	})

	for tick := range 3 {
		link.enqueue(context.Background(), simpleCommandTo(fixtureA.world.address, hungAddress, tick))
		link.enqueue(context.Background(), simpleCommandTo(fixtureA.world.address, fixtureB.world.address, tick))
		start := time.Now()
		link.drain()
		assert.Less(t, time.Since(start), 500*time.Millisecond, "drain waited on a send")
	}

	unhang()
	var got []command.Command
	require.Eventually(t, func() bool {
		fixtureB.world.commands.Drain()
		cmds, err := fixtureB.world.commands.Get(fixtureB.commandID)
		require.NoError(t, err)
		got = append(got, cmds...)
		return len(got) == 3
	}, 5*time.Second, 10*time.Millisecond)
	for i, cmd := range got {
		assert.Equal(t, testutils.SimpleCommand{Value: i}, cmd.Payload)
	}
}
