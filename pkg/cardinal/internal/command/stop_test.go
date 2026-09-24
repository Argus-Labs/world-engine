package command_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/testutils"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Stopped-flag tests
// -------------------------------------------------------------------------------------------------
// The run loop is the only drainer of the command queue. Once it exits, no accepted command will
// ever be processed, so Enqueue must reject new commands (SetStopped) instead of falsely
// acknowledging them. These tests pin the stopped flag's guarantees: Enqueue is rejected after
// SetStopped, the pending count is accurate, the flag is idempotent, the pending commands are not
// dropped by SetStopped, and — under concurrency — the stopped check is atomic with the enqueue so
// no command is silently dropped (run with -race).
// -------------------------------------------------------------------------------------------------

func TestCommand_SetStoppedRejectsAndCounts(t *testing.T) {
	t.Parallel()

	impl := command.NewManager()
	_, err := impl.Register(testutils.SimpleCommand{}.Name(), command.NewQueue[testutils.SimpleCommand]())
	require.NoError(t, err)

	// Before stop, enqueues succeed.
	require.NoError(t, impl.Enqueue(simpleCmd(1)))
	require.NoError(t, impl.Enqueue(simpleCmd(2)))

	// SetStopped reports the two pending commands and closes the gate.
	require.Equal(t, 2, impl.SetStopped(), "SetStopped must report the number of pending commands")

	// Idempotent: a second call keeps the gate closed and returns 0.
	require.Equal(t, 0, impl.SetStopped(), "SetStopped is idempotent")

	// After stop, enqueue is rejected with ErrStopped and the command is not stored.
	require.ErrorIs(t, impl.Enqueue(simpleCmd(3)), command.ErrStopped)

	// SetStopped does not drop the pending commands; they are still recoverable via Drain.
	all := impl.Drain()
	require.Len(t, all, 2)
	values := []int{
		all[0].Payload.(testutils.SimpleCommand).Value,
		all[1].Payload.(testutils.SimpleCommand).Value,
	}
	assert.ElementsMatch(t, []int{1, 2}, values)

	// The gate stays closed after a drain: no enqueue can resurrect acceptance.
	require.ErrorIs(t, impl.Enqueue(simpleCmd(4)), command.ErrStopped)
}

// TestCommand_SetStoppedAtomicWithEnqueue proves the stopped check and the per-queue append are
// atomic: under a burst of concurrent enqueues with SetStopped firing mid-burst, every enqueue is
// either accepted (and counted by SetStopped, and still present in a later Drain) or rejected with
// ErrStopped — never silently dropped. Run with -race to also catch data races.
func TestCommand_SetStoppedAtomicWithEnqueue(t *testing.T) {
	t.Parallel()

	const (
		workers   = 32
		perWorker = 200
		stopAt    = 100
	)

	impl := command.NewManager()
	_, err := impl.Register(testutils.SimpleCommand{}.Name(), command.NewQueue[testutils.SimpleCommand]())
	require.NoError(t, err)

	var (
		wg        sync.WaitGroup
		start     = make(chan struct{})
		succeeded atomic.Int64
		rejected  atomic.Int64
		stopN     atomic.Int64
		stopOnce  sync.Once
	)

	for range workers {
		wg.Go(func() {
			prng := testutils.NewRand(t)
			<-start
			for i := range perWorker {
				err := impl.Enqueue(simpleCmd(prng.IntN(1_000_000)))
				switch {
				case err == nil:
					succeeded.Add(1)
				case errors.Is(err, command.ErrStopped):
					rejected.Add(1)
				default:
					t.Errorf("unexpected enqueue error: %v", err)
					return
				}
				if i == stopAt {
					stopOnce.Do(func() { stopN.Store(int64(impl.SetStopped())) })
				}
			}
		})
	}
	close(start)
	wg.Wait()

	totalSucceeded := int(succeeded.Load())
	totalRejected := int(rejected.Load())

	// Every enqueue attempt is either accepted or rejected with ErrStopped.
	require.Equal(t, workers*perWorker, totalSucceeded+totalRejected,
		"every enqueue must be classified as accepted or ErrStopped")

	// Every accepted enqueue happened before SetStopped set the flag (the two share stop.mu), so the
	// count SetStopped returned equals the number of accepted enqueues (the queue started empty and
	// no concurrent drain runs in this test).
	require.Equal(t, totalSucceeded, int(stopN.Load()),
		"SetStopped count must equal the number of accepted enqueues")

	// The drained commands equal the accepted enqueues: nothing was silently dropped.
	require.Len(t, impl.Drain(), totalSucceeded, "drained count must match accepted enqueues")
}

// simpleCmd builds a wire-encoded SimpleCommand ready for Enqueue.
func simpleCmd(value int) *iscv1.Command {
	payload := testutils.SimpleCommand{Value: value}
	return &iscv1.Command{
		Name:    payload.Name(),
		Address: &microv1.ServiceAddress{},
		Persona: &iscv1.Persona{Id: "persona"},
		Payload: schema.Marshal(payload),
	}
}
