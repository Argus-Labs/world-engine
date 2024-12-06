package gamestate

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"
	"go.opentelemetry.io/otel/codes"
)

// The world tick must be updated in the same atomic transaction as all the state changes
// associated with that tick. This means the manager here must also implement the TickStore interface.
var _ TickStorage = &EntityCommandBuffer{}

// GetLastFinalizedTick returns the last tick that was successfully finalized.
// If the latest finalized tick is 0, it means that no tick has been finalized yet.
func (m *EntityCommandBuffer) GetLastFinalizedTick() (uint64, error) {
	ctx := context.Background()

	tick, err := m.dbStorage.GetUInt64(ctx, storageLastFinalizedTickKey())
	if err != nil {
		// If the returned error is redis.Nil, it means that the key does not exist yet. In this case, we can infer
		// that the latest finalized tick is 0. If the return is not redis.Nil, it means that an actual error occurred.
		if errors.Is(err, redis.Nil) {
			tick = 0
		} else {
			return 0, eris.Wrap(err, "failed to get latest finalized tick")
		}
	}

	return tick, nil
}

// FinalizeTick combines all pending state changes into a single multi/exec redis transactions and commits them
// to the DB.
func (m *EntityCommandBuffer) FinalizeTick(ctx context.Context) error {
	ctx, span := m.tracer.Start(ctx, "ecb.tick.finalize")
	defer span.End()

	pipe, err := m.makePipeOfRedisCommands(ctx)
	if err != nil {
		span.SetStatus(codes.Error, eris.ToString(err, true))
		span.RecordError(err)
		return eris.Wrap(err, "failed to make redis commands pipe")
	}

	if err := pipe.Incr(ctx, storageLastFinalizedTickKey()); err != nil {
		span.SetStatus(codes.Error, eris.ToString(err, true))
		span.RecordError(err)
		return eris.Wrap(err, "failed to increment latest finalized tick")
	}

	if err := pipe.EndTransaction(ctx); err != nil {
		span.SetStatus(codes.Error, eris.ToString(err, true))
		span.RecordError(err)
		return eris.Wrap(err, "failed to end transaction")
	}

	m.pendingArchIDs = nil

	if err := m.DiscardPending(); err != nil {
		span.SetStatus(codes.Error, eris.ToString(err, true))
		span.RecordError(err)
		return eris.Wrap(err, "failed to discard pending state changes")
	}

	return nil
}
