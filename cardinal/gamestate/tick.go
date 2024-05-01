package gamestate

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"pkg.world.dev/world-engine/cardinal/statsd"
)

// The engine tick must be updated in the same atomic transaction as all the state changes
// associated with that tick. This means the manager here must also implement the TickStore interface.
var _ TickStorage = &EntityCommandBuffer{}

// GetTickNumber returns the last tick that was started and the last tick that was ended. If start == end, it means
// the last tick that was attempted completed successfully. If start != end, it means a tick was started but did not
// complete successfully; Recover must be used to recover the pending transactions so the previously started tick can
// be completed.
func (m *EntityCommandBuffer) GetTickNumber(ctx context.Context) (curr uint64, err error) {
	curr, err = m.dbStorage.GetUInt64(ctx, storageCurrentTickKey())
	err = eris.Wrap(err, "")
	if eris.Is(eris.Cause(err), redis.Nil) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	return curr, nil
}

// FinalizeTick combines all pending state changes into a single multi/exec redis transactions and commits them
// to the DB.
func (m *EntityCommandBuffer) FinalizeTick(ctx context.Context) error {
	var span tracer.Span
	span, ctx = tracer.StartSpanFromContext(ctx, "tick.span.finalize")
	defer func() {
		span.Finish()
	}()
	makePipeStartTime := time.Now()
	pipe, err := m.makePipeOfRedisCommands(ctx)
	if err != nil {
		return err
	}
	if err = pipe.Incr(ctx, storageCurrentTickKey()); err != nil {
		return eris.Wrap(err, "")
	}
	statsd.EmitTickStat(makePipeStartTime, "pipe_make")
	flushStartTime := time.Now()
	err = pipe.EndTransaction(ctx)
	statsd.EmitTickStat(flushStartTime, "pipe_exec")
	if err != nil {
		return eris.Wrap(err, "")
	}

	m.pendingArchIDs = nil
	return m.DiscardPending()
}
