package transport

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/argus-labs/world-engine/pkg/testutils"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInterShard_StopSendsFlushedInOrder checks order across flushes, and that Stop waits for every send.
func TestInterShard_StopSendsFlushedInOrder(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	fixtureA := newTransportFixture(t, prng, true)
	fixtureB := newTransportFixture(t, prng, true)

	const flushes, perFlush = 5, 4
	for flush := range flushes {
		for i := range perFlush {
			fixtureA.tr.Enqueue(context.Background(), fixtureB.address, testutils.SimpleCommand{Value: flush*perFlush + i})
		}
		fixtureA.tr.Flush()
	}
	require.NoError(t, fixtureA.tr.Stop(context.Background()))

	// Stop returned only after B accepted every send, so one take sees them all.
	cmds := fixtureB.received.take()
	require.Len(t, cmds, flushes*perFlush)
	for i, cmd := range cmds {
		assert.Equal(t, testutils.SimpleCommand{Value: i}, decodeSimpleCommand(t, cmd))
	}
}

// TestInterShard_ConcurrentEnqueueFlush checks that goroutines enqueuing and flushing at once lose no
// command, and that each goroutine's commands reach each target in the order it enqueued them.
func TestInterShard_ConcurrentEnqueueFlush(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	fixtureA := newTransportFixture(t, prng, true)
	targets := []*transportFixture{newTransportFixture(t, prng, true), newTransportFixture(t, prng, true)}

	const goroutines, perGoroutine = 8, 50
	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Go(func() {
			for i := range perGoroutine {
				to := targets[i%len(targets)].address
				fixtureA.tr.Enqueue(context.Background(), to, testutils.SimpleCommand{Value: g*perGoroutine + i})
				if i%3 == 0 {
					fixtureA.tr.Flush()
				}
			}
			fixtureA.tr.Flush()
		})
	}
	wg.Wait()
	require.NoError(t, fixtureA.tr.Stop(context.Background()))

	total := 0
	for _, target := range targets {
		last := make(map[int]int) // Goroutine to its last value seen at this target
		for _, cmd := range target.received.take() {
			value := decodeSimpleCommand(t, cmd).Value
			g := value / perGoroutine
			if prev, ok := last[g]; ok {
				assert.Greater(t, value, prev, "goroutine %d's commands arrived out of order", g)
			}
			last[g] = value
			total++
		}
	}
	assert.Equal(t, goroutines*perGoroutine, total)
}

// TestInterShard_HungTargetIsolated checks that a target that never acks neither blocks Flush nor delays
// commands to a healthy target.
func TestInterShard_HungTargetIsolated(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	fixtureA := newTransportFixture(t, prng, true)
	fixtureB := newTransportFixture(t, prng, true)

	hungAddress := RandServiceAddress(prng)
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	hungClient := NewTestClient(t)
	hung, err := micro.NewService(hungClient, hungAddress, &telemetry.Telemetry{Logger: zerolog.Nop()})
	require.NoError(t, err)
	require.NoError(t, hung.AddGroup("command").AddEndpoint(testutils.SimpleCommand{}.Name(),
		func(_ context.Context, req *micro.Request) *micro.Response {
			select {
			case entered <- struct{}{}:
			default:
			}
			<-release
			return micro.NewSuccessResponse(req, nil)
		}))
	require.NoError(t, hungClient.Flush())

	t.Cleanup(func() {
		close(release)
		_ = fixtureA.tr.Stop(context.Background())
		_ = hung.Close()
	})

	for flush := range 3 {
		fixtureA.tr.Enqueue(context.Background(), hungAddress, testutils.SimpleCommand{Value: flush})
		fixtureA.tr.Enqueue(context.Background(), fixtureB.address, testutils.SimpleCommand{Value: flush})
		start := time.Now()
		fixtureA.tr.Flush()
		assert.Less(t, time.Since(start), 500*time.Millisecond, "flush waited on a send")
	}

	// B receives all three while the hung target still holds its first command.
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("hung target never received a command")
	}
	got := awaitCommands(t, fixtureB, 3)
	for i, cmd := range got {
		assert.Equal(t, testutils.SimpleCommand{Value: i}, decodeSimpleCommand(t, cmd))
	}
}

// TestInterShard_SenderExitsWhenIdle checks that once a target's commands are sent, its sender and map
// entry are gone, and that a later Flush starts a new one.
func TestInterShard_SenderExitsWhenIdle(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	fixtureA := newTransportFixture(t, prng, true)
	fixtureB := newTransportFixture(t, prng, true)
	link := fixtureA.tr.interShard

	senders := func() int {
		link.mu.Lock()
		defer link.mu.Unlock()
		return len(link.senders)
	}

	for round := range 2 {
		fixtureA.tr.Enqueue(context.Background(), fixtureB.address, testutils.SimpleCommand{Value: round})
		fixtureA.tr.Flush()
		cmds := awaitCommands(t, fixtureB, 1)
		assert.Equal(t, testutils.SimpleCommand{Value: round}, decodeSimpleCommand(t, cmds[0]))
		require.Eventually(t, func() bool { return senders() == 0 }, 5*time.Second, 10*time.Millisecond)
	}
}

// TestInterShard_RejectsNonShardPersona checks that a command whose persona is not a shard address is
// rejected before it reaches the handler.
func TestInterShard_RejectsNonShardPersona(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	fixture := newTransportFixture(t, prng, true)
	payload := testutils.SimpleCommand{Value: 1}
	_, err := NewTestClient(t).Request(context.Background(), fixture.address, "command."+payload.Name(),
		&iscv1.Command{
			Name:    payload.Name(),
			Address: fixture.address,
			Persona: &iscv1.Persona{Id: "not-a-shard"},
			Payload: payload.MarshalWire(),
		})
	require.Error(t, err)
	assert.Empty(t, fixture.received.take())
}
