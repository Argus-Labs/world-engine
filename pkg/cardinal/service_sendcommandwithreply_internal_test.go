package cardinal

import (
	"context"
	"encoding/binary"
	"errors"
	"math/rand/v2"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

// -------------------------------------------------------------------------------------------------
// SendCommandWithReply TOCTOU regression tests
// -------------------------------------------------------------------------------------------------
// SendCommandWithReply must register its reply waiter BEFORE enqueuing the command. Enqueueing
// first opens a window in which the tick loop can drain the command, emit the reply, and look up
// replyWaiters before the waiter exists — dropping the reply and deadlocking the client until its
// context times out.
//
// The first test deterministically reproduces the worst-case interleaving (a tick that fires the
// instant the command becomes eligible) by injecting an inline tick into the command queue's
// Enqueue; the buggy ordering hangs/fails, the fixed ordering delivers the reply. The second test
// exercises the real handler against a live tick loop with concurrent callers and per-call unique
// event names so a dropped reply cannot be masked by another caller's reply.
// -------------------------------------------------------------------------------------------------

const (
	scwrReplyTimeout         = 2 * time.Second
	scwrStressPerCallTimeout = 2 * time.Second
	scwrStressTickRate       = 1000.0
	scwrStressSnapshotRate   = 1000
	scwrStressWorkers        = 8
	scwrStressCallsPerWorker = 250
	scwrStressRunStopTimeout = 5 * time.Second
	scwrCommandName          = "scwr_reply_command"
	scwrEventNamePrefix      = "scwr_reply_event_"
)

// replyCommand is a self-contained command payload whose reply carries a unique ReplyID so each
// SendCommandWithReply caller can wait on a distinct event name. Unique names prevent the reply
// fan-out (publishDefaultEvent sends every event to every waiter registered for that name) from
// masking a dropped reply with another caller's reply reaching the waiter late.
type replyCommand struct {
	ReplyID uint64
	Value   int64
}

func (replyCommand) Name() string { return scwrCommandName }

func (c replyCommand) SizeWire() int { return 16 }

func (c replyCommand) AppendWire(b []byte) []byte {
	b = binary.LittleEndian.AppendUint64(b, c.ReplyID)
	return binary.LittleEndian.AppendUint64(b, uint64(c.Value))
}

func (replyCommand) UnmarshalWire(b []byte) (any, error) {
	if len(b) < 16 {
		return nil, eris.New("replyCommand: malformed wire bytes")
	}
	return replyCommand{
		ReplyID: binary.LittleEndian.Uint64(b[:8]),
		Value:   int64(binary.LittleEndian.Uint64(b[8:16])),
	}, nil
}

// replyEvent echoes the command's ReplyID. Its Name() is per-instance (encodes ReplyID), so each
// caller waits on a distinct event — a dropped reply cannot be masked by another caller's reply
// reaching the waiter.
type replyEvent struct {
	ReplyID uint64
	Value   int64
}

func (e replyEvent) Name() string {
	return scwrEventNamePrefix + strconv.FormatUint(e.ReplyID, 10)
}

func (e replyEvent) SizeWire() int { return 16 }

func (e replyEvent) AppendWire(b []byte) []byte {
	b = binary.LittleEndian.AppendUint64(b, e.ReplyID)
	return binary.LittleEndian.AppendUint64(b, uint64(e.Value))
}

func (replyEvent) UnmarshalWire(b []byte) (any, error) {
	if len(b) < 16 {
		return nil, eris.New("replyEvent: malformed wire bytes")
	}
	return replyEvent{
		ReplyID: binary.LittleEndian.Uint64(b[:8]),
		Value:   int64(binary.LittleEndian.Uint64(b[8:16])),
	}, nil
}

// replySystem drains replyCommand inputs every tick and broadcasts a replyEvent echoing each
// command's ReplyID/Value — mirroring how a real game system emits a reply to SendCommandWithReply.
type replySystem struct {
	BaseSystemState
	Command WithCommand[replyCommand]
	Events  WithEvent[replyEvent]
}

func (s *replySystem) Run() {
	for ctx := range s.Command.Iter() {
		s.Events.Broadcast(replyEvent{ReplyID: ctx.Payload.ReplyID, Value: ctx.Payload.Value})
	}
}

// tickingReplyQueue wraps a real command queue and, after a successful Enqueue, synchronously runs
// an injected "tick". This deterministically reproduces the worst-case enqueue-then-tick
// interleaving: a tick that fires the instant the command becomes eligible (before Enqueue returns
// to the caller). If SendCommandWithReply registered its waiter before Enqueue (the fix), the tick
// finds it and delivers the reply; otherwise (the bug) the reply is dropped and the client blocks
// until its context times out.
type tickingReplyQueue struct {
	command.Queue
	tick func()
}

func (q *tickingReplyQueue) Enqueue(cmd *iscv1.Command) error {
	if err := q.Queue.Enqueue(cmd); err != nil {
		return err
	}
	q.tick()
	return nil
}

// failingQueue rejects every Enqueue, used to verify the deferred removeReplyWaiter cleans up the
// waiter on the Enqueue error path (no leak, and no dropped reply since nothing is processed before
// the failure).
type failingQueue struct {
	command.Queue
}

func (q *failingQueue) Enqueue(*iscv1.Command) error {
	return eris.New("simulated enqueue failure")
}

// scwrAuthCtx returns parent bound to the given user, the way authn.SetInfo does for a real
// authenticated request. SendCommandWithReply reads the user via UserFromContext and uses the
// context's Done channel as its reply-deadline, so the parent must carry the timeout.
func scwrAuthCtx(parent context.Context, userID string) context.Context {
	return authn.SetInfo(parent, &User{ID: userID})
}

// newSCWRFixture builds a bare World whose KindDefault event handler is publishDefaultEvent, with
// the provided command queue registered under the simple_command name. The handler is exercised
// directly (no HTTP server), so the service is created but not initialized.
func newSCWRFixture(
	t *testing.T,
	prng *rand.Rand,
	queueFn func(*World) command.Queue,
) (*service, *World) {
	t.Helper()
	w := &World{
		world:    ecs.NewWorld(),
		commands: command.NewManager(),
		events:   event.NewManager(1024),
		address:  RandServiceAddress(prng),
		tel: telemetry.Telemetry{
			Logger: zerolog.Nop(),
			Tracer: noop.NewTracerProvider().Tracer("test"),
		},
	}
	svc := newService(w, AuthModeDev, "")
	svc.registerCommandHandler(testutils.SimpleCommand{}.Name())
	w.service = svc
	w.events.RegisterHandler(event.KindDefault, svc.publishDefaultEvent)
	_, err := w.commands.Register(testutils.SimpleCommand{}.Name(), queueFn(w))
	require.NoError(t, err)
	return svc, w
}

func TestService_SendCommandWithReply_WaiterRegisteredBeforeEnqueue(t *testing.T) {
	t.Parallel()

	t.Run("reply delivered when tick races enqueue", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		replyValue := prng.Int()
		var dispatchErr error
		svc, w := newSCWRFixture(t, prng, func(world *World) command.Queue {
			return &tickingReplyQueue{
				Queue: command.NewQueue[testutils.SimpleCommand](),
				tick: func() {
					// Emit the reply event named "simple_event" and dispatch it immediately. This
					// models a tick that drains the command, runs a system that emits the reply,
					// and flushes events — all before Enqueue returns to the caller.
					world.events.Enqueue(event.Event{
						Kind:    event.KindDefault,
						Payload: testutils.SimpleEvent{Value: replyValue},
					})
					dispatchErr = world.events.Dispatch()
				},
			}
		})

		userID := testutils.RandString(prng, 8)
		cmdPb := &iscv1.Command{
			Name:    testutils.SimpleCommand{}.Name(),
			Address: w.address,
			Persona: &iscv1.Persona{Id: "client-provided-persona"},
			Payload: testutils.SimpleCommand{Value: prng.IntN(1_000_000)}.MarshalWire(),
		}

		ctx, cancel := context.WithTimeout(context.Background(), scwrReplyTimeout)
		defer cancel()
		resp, err := svc.SendCommandWithReply(
			scwrAuthCtx(ctx, userID),
			connect.NewRequest(&cardinalv1.SendCommandWithReplyRequest{
				Command:   cmdPb,
				EventName: testutils.SimpleEvent{}.Name(),
			}),
		)
		require.NoError(t, err, "reply should be delivered before the context deadline; a "+
			"timeout indicates the TOCTOU regression (waiter registered after enqueue, reply dropped)")
		require.NoError(t, dispatchErr, "inline tick dispatch should succeed")
		require.NotNil(t, resp)
		gotEvent := resp.Msg.GetEvent()
		require.Equal(t, testutils.SimpleEvent{}.Name(), gotEvent.GetName())
		decoded, derr := testutils.SimpleEvent{}.UnmarshalWire(gotEvent.GetPayload())
		require.NoError(t, derr)
		assert.Equal(t, testutils.SimpleEvent{Value: replyValue}, decoded)
	})

	t.Run("enqueue failure removes waiter without leak", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		svc, w := newSCWRFixture(t, prng, func(*World) command.Queue {
			return &failingQueue{Queue: command.NewQueue[testutils.SimpleCommand]()}
		})

		eventName := testutils.SimpleEvent{}.Name()
		userID := testutils.RandString(prng, 8)
		cmdPb := &iscv1.Command{
			Name:    testutils.SimpleCommand{}.Name(),
			Address: w.address,
			Persona: &iscv1.Persona{Id: "client-provided-persona"},
			Payload: testutils.SimpleCommand{Value: 42}.MarshalWire(),
		}

		ctx, cancel := context.WithTimeout(context.Background(), scwrReplyTimeout)
		defer cancel()
		_, err := svc.SendCommandWithReply(
			scwrAuthCtx(ctx, userID),
			connect.NewRequest(&cardinalv1.SendCommandWithReplyRequest{
				Command:   cmdPb,
				EventName: eventName,
			}),
		)
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

		// The deferred removeReplyWaiter must have cleaned up the waiter registered before the
		// failed Enqueue; otherwise waiters accumulate on every failing call (a leak).
		svc.mu.RLock()
		_, leak := svc.replyWaiters[eventName]
		svc.mu.RUnlock()
		assert.False(t, leak, "reply waiter leaked after enqueue failure")
	})
}

// TestService_SendCommandWithReply_ReplyDeliveredUnderHotTickLoop exercises the real handler
// against a live tick loop with concurrent callers. Each caller waits on a unique event name
// (encoded by ReplyID), so a dropped reply cannot be masked by another caller's reply reaching
// the waiter late. With the fix, every reply is delivered and round-trips to the matching
// ReplyID/Value. Before the fix, replies dropped by the enqueue-before-waiter race surface as
// context-deadline losses.
func TestService_SendCommandWithReply_ReplyDeliveredUnderHotTickLoop(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)
	w := newSCWRWorld(t, prng)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- w.run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-runDone:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("world.run returned unexpected error: %v", err)
			}
		case <-time.After(scwrStressRunStopTimeout):
			t.Errorf("world.run did not stop within %s", scwrStressRunStopTimeout)
		}
	})

	var (
		wg         sync.WaitGroup
		losses     atomic.Int64
		mismatches atomic.Int64
	)
	totalCalls := scwrStressWorkers * scwrStressCallsPerWorker
	wg.Add(scwrStressWorkers)
	for worker := range scwrStressWorkers {
		go func() {
			defer wg.Done()
			for i := range scwrStressCallsPerWorker {
				replyID := uint64(worker)*uint64(scwrStressCallsPerWorker) + uint64(i)
				value := int64(replyID)
				eventName := scwrEventNamePrefix + strconv.FormatUint(replyID, 10)
				cmdPb := &iscv1.Command{
					Name:    scwrCommandName,
					Address: w.address,
					Persona: &iscv1.Persona{Id: "stress-user"},
					Payload: schema.Marshal(replyCommand{ReplyID: replyID, Value: value}),
				}
				callCtx, callCancel := context.WithTimeout(context.Background(), scwrStressPerCallTimeout)
				resp, callErr := w.service.SendCommandWithReply(
					scwrAuthCtx(callCtx, "stress-user"),
					connect.NewRequest(&cardinalv1.SendCommandWithReplyRequest{
						Command:   cmdPb,
						EventName: eventName,
					}),
				)
				callCancel()
				if callErr != nil {
					losses.Add(1)
					continue
				}
				decoded, derr := replyEvent{}.UnmarshalWire(resp.Msg.GetEvent().GetPayload())
				if derr != nil {
					mismatches.Add(1)
					continue
				}
				re, ok := decoded.(replyEvent)
				if !ok || re.ReplyID != replyID || re.Value != value {
					mismatches.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	require.Zero(t, losses.Load(), "%d/%d replies were lost (context deadline) under the hot "+
		"tick loop — indicates the TOCTOU regression (waiter registered after enqueue)",
		losses.Load(), totalCalls)
	require.Zero(t, mismatches.Load(), "%d/%d replies did not round-trip to the matching "+
		"ReplyID/Value — reply routing is corrupted", mismatches.Load(), totalCalls)
}

// newSCWRWorld builds a bare World with a Nop snapshot store (so w.run can restore/persist) and a
// hot tick loop, registers the replySystem, and wires publishDefaultEvent as the KindDefault
// handler. The service is created but not initialized — the handler is exercised directly.
func newSCWRWorld(t *testing.T, prng *rand.Rand) *World {
	t.Helper()
	nopStorage := snapshot.NewNopStorage()
	w := &World{
		world:           ecs.NewWorld(),
		commands:        command.NewManager(),
		events:          event.NewManager(1024),
		address:         RandServiceAddress(prng),
		options:         WorldOptions{TickRate: scwrStressTickRate, SnapshotRate: scwrStressSnapshotRate},
		snapshotStorage: nopStorage,
		snapshotWriter:  snapshot.NewSyncWriter(nopStorage, zerolog.Nop()),
		tel: telemetry.Telemetry{
			Logger: zerolog.Nop(),
			Tracer: noop.NewTracerProvider().Tracer("test"),
		},
	}
	w.service = newService(w, AuthModeDev, "")
	w.events.RegisterHandler(event.KindDefault, w.service.publishDefaultEvent)
	w.events.RegisterHandler(event.KindInterShardCommand, w.service.publishInterShardCommand)
	require.NotPanics(t, func() { w.RegisterSystemV2(&replySystem{}) })
	return w
}
