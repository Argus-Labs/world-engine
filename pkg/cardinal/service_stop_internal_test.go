package cardinal

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// SendCommand / SendCommandWithReply shutdown-gating tests
// -------------------------------------------------------------------------------------------------
// The run loop is the only drainer of the command queue. Once it has exited, a SendCommand that
// still succeeds is a false acknowledgement: the command is enqueued but never processed. The fix
// gates Enqueue on a stopped flag set at run-loop exit (and shutdown), so the service rejects
// sends with Unavailable instead. These tests pin that contract at the handler level and
// reproduce the production ordering (run loop exits while the service is still callable).
// -------------------------------------------------------------------------------------------------

func TestService_SendCommandRejectedWhenStopped(t *testing.T) {
	t.Parallel()

	t.Run("SendCommand returns Unavailable", func(t *testing.T) {
		t.Parallel()
		fixture := newServiceFixture(t, testutils.NewRand(t), false)

		// Sanity: a command is accepted while the world is running.
		_, err := fixture.svc.SendCommand(
			serviceTestContext("user"),
			connect.NewRequest(&cardinalv1.SendCommandRequest{
				Command: simpleServiceCommand(fixture.world.address, 1),
			}),
		)
		require.NoError(t, err)

		// Simulate the run loop having exited.
		fixture.world.commands.SetStopped()

		_, err = fixture.svc.SendCommand(
			serviceTestContext("user"),
			connect.NewRequest(&cardinalv1.SendCommandRequest{
				Command: simpleServiceCommand(fixture.world.address, 2),
			}),
		)
		require.Error(t, err)
		assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
		assert.Contains(t, err.Error(), "shutting down")
	})

	t.Run("SendCommandWithReply returns Unavailable without registering a waiter", func(t *testing.T) {
		t.Parallel()
		fixture := newServiceFixture(t, testutils.NewRand(t), false)

		fixture.world.commands.SetStopped()

		_, err := fixture.svc.SendCommandWithReply(
			serviceTestContext("user"),
			connect.NewRequest(&cardinalv1.SendCommandWithReplyRequest{
				Command:   simpleServiceCommand(fixture.world.address, 3),
				EventName: "reply-event",
			}),
		)
		require.Error(t, err)
		assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))

		// The handler must return before registering a reply waiter, so none is leaked.
		fixture.svc.mu.Lock()
		empty := len(fixture.svc.replyWaiters) == 0
		fixture.svc.mu.Unlock()
		assert.True(t, empty, "no reply waiter should be registered when the world is stopped")
	})

	t.Run("non-stopped enqueue errors keep InvalidArgument", func(t *testing.T) {
		t.Parallel()
		fixture := newServiceFixture(t, testutils.NewRand(t), false)

		// An unregistered command name reaches Enqueue and fails with "unregistered command".
		// This is not the stopped path, so it must stay InvalidArgument (regression guard).
		cmdPb := &iscv1.Command{
			Name:    "definitely-not-a-registered-command",
			Address: fixture.world.address,
			Persona: &iscv1.Persona{Id: "client"},
			Payload: testutils.SimpleCommand{Value: 9}.MarshalWire(),
		}
		_, err := fixture.svc.SendCommand(
			serviceTestContext("user"),
			connect.NewRequest(&cardinalv1.SendCommandRequest{Command: cmdPb}),
		)
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})
}

// TestService_SendCommandRejectedAfterRunLoopExits reproduces the production shutdown ordering:
// the run loop (the only drainer of the command queue) exits while the service is still callable,
// and a SendCommand issued in that window must be rejected with Unavailable instead of falsely
// acknowledged.
//
// Not parallel: newRunnableWorldFixture uses t.Setenv to suppress logs (same pattern as the DST
// and e2e fixtures).
func TestService_SendCommandRejectedAfterRunLoopExits(t *testing.T) {
	w, _ := newRunnableWorldFixture(t)

	// Start the run loop (the only drainer of the command queue).
	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- w.run(ctx) }()
	t.Cleanup(cancel) // ensure ctx is cancelled if an assertion fails early

	// While the run loop is alive, SendCommand succeeds (the still-living drainer is the
	// condition the bug relied on being absent).
	cmdPb := &iscv1.Command{
		Name:    testutils.SimpleCommand{}.Name(),
		Address: w.address,
		Persona: &iscv1.Persona{Id: "client"},
		Payload: testutils.SimpleCommand{Value: 42}.MarshalWire(),
	}
	_, err := w.service.SendCommand(
		serviceTestContext("user"),
		connect.NewRequest(&cardinalv1.SendCommandRequest{Command: cmdPb}),
	)
	require.NoError(t, err)

	// Cancel the run loop and wait for it to return. Its exit defer sets the command manager
	// stopped, closing the window the bug relied on. (We do not inspect the per-type buffer from
	// here — Get is unlocked and the run loop mutates it; the existing SendCommand/WithCommand
	// tests cover draining.)
	cancel()
	require.ErrorIs(t, <-runErr, context.Canceled)

	// The service is still callable, but the command manager is stopped: a send now would be
	// falsely acknowledged without the fix. Assert it is rejected with Unavailable instead.
	_, err = w.service.SendCommand(
		serviceTestContext("user"),
		connect.NewRequest(&cardinalv1.SendCommandRequest{Command: cmdPb}),
	)
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "shutting down")
}

// TestService_SendCommandSucceedsBeforeRunLoopExits is the paired control: a SendCommand issued
// while the run loop is alive is accepted (and not rejected), confirming the stopped gate is not
// over-triggering during normal operation.
//
// Not parallel: newRunnableWorldFixture uses t.Setenv to suppress logs.
func TestService_SendCommandSucceedsBeforeRunLoopExits(t *testing.T) {
	w, _ := newRunnableWorldFixture(t)

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- w.run(ctx) }()
	t.Cleanup(cancel)

	cmdPb := &iscv1.Command{
		Name:    testutils.SimpleCommand{}.Name(),
		Address: w.address,
		Persona: &iscv1.Persona{Id: "client"},
		Payload: testutils.SimpleCommand{Value: 7}.MarshalWire(),
	}

	// Repeated sends while the loop is alive all succeed.
	for range 5 {
		_, err := w.service.SendCommand(
			serviceTestContext("user"),
			connect.NewRequest(&cardinalv1.SendCommandRequest{Command: cmdPb}),
		)
		require.NoError(t, err)
	}

	cancel()
	require.ErrorIs(t, <-runErr, context.Canceled)
}

// TestService_StopCommandsGatesSends exercises the World.stopCommands wrapper (the single function
// called from the run loop's exit defer and from shutdown) end to end: after it runs, the service
// rejects SendCommand with Unavailable. It also confirms that pending commands being dropped is
// non-fatal (the wrapper only logs).
func TestService_StopCommandsGatesSends(t *testing.T) {
	t.Parallel()
	fixture := newServiceFixture(t, testutils.NewRand(t), false)

	// A pending (un-ticked) command plus a stopped world is the loss case; it must not panic.
	require.NoError(t, fixture.world.commands.Enqueue(simpleServiceCommand(fixture.world.address, 1)))
	fixture.world.stopCommands()

	_, err := fixture.svc.SendCommand(
		serviceTestContext("user"),
		connect.NewRequest(&cardinalv1.SendCommandRequest{Command: simpleServiceCommand(fixture.world.address, 2)}),
	)
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
}

// newRunnableWorldFixture builds a real World (Nop snapshot, dev auth) with SimpleCommand
// registered, suitable for driving the run loop directly. Mirrors the DST fixture's NewWorld use.
func newRunnableWorldFixture(t *testing.T) (*World, command.ID) {
	t.Helper()
	t.Setenv("LOG_LEVEL", "disabled")

	debug, pprof := false, false
	w, err := NewWorld(WorldOptions{
		Region:              "test",
		Organization:        "test",
		Project:             "test",
		ShardID:             "0",
		TickRate:            50,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        1,
		Debug:               &debug,
		Pprof:               &pprof,
		AuthMode:            AuthModeDev,
	})
	require.NoError(t, err)

	cmdID, err := w.commands.Register(testutils.SimpleCommand{}.Name(), command.NewQueue[testutils.SimpleCommand]())
	require.NoError(t, err)

	return w, cmdID
}

// simpleServiceCommand builds a wire-encoded SimpleCommand request body addressed to the shard.
func simpleServiceCommand(address *micro.ServiceAddress, value int) *iscv1.Command {
	return &iscv1.Command{
		Name:    testutils.SimpleCommand{}.Name(),
		Address: address,
		Persona: &iscv1.Persona{Id: "client"},
		Payload: testutils.SimpleCommand{Value: value}.MarshalWire(),
	}
}
