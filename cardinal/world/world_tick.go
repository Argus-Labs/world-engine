package world

import (
	"context"
	"time"

	"github.com/goccy/go-json"
	"github.com/rotisserie/eris"
	"go.opentelemetry.io/otel/codes"
	ddotel "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/opentelemetry"
	ddtracer "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"pkg.world.dev/world-engine/cardinal/tick"
	"pkg.world.dev/world-engine/cardinal/types"
)

const (
	cacheSize = 10 * 1024 * 1024
	cacheTTL  = 5 * 60 // 5 minutes
)

var ErrInvalidReceiptTxHash = eris.New("invalid receipt tx hash")

func (w *World) LastFinalizedTick() int64 {
	return w.lastFinalizedTickID
}

// PrepareTick creates a new proposal for the next tick.
func (w *World) PrepareTick(txs types.TxMap) tick.Proposal {
	return tick.Proposal{
		ID:        w.lastFinalizedTickID + 1,
		Timestamp: time.Now().UnixMilli(),
		Namespace: w.namespace,
		Txs:       txs,
	}
}

// PrepareSyncTick creates a new proposal for the next tick based on historical tick data obtained from syncing.
func (w *World) PrepareSyncTick(id int64, timestamp int64, txs types.TxMap) tick.Proposal {
	return tick.Proposal{
		ID:        id,
		Timestamp: timestamp,
		Namespace: w.namespace,
		Txs:       txs,
	}
}

func (w *World) ApplyTick(ctx context.Context, proposal *tick.Proposal) (*tick.Tick, error) {
	ctx, span := w.tracer.Start(ddotel.ContextWithStartOptions(ctx, ddtracer.Measured()), "world.tick")
	defer span.End()

	// This defer is here to catch any panics that occur during the tick. It will log the current tick and the
	// current system that is running.
	defer func() {
		if r := recover(); r != nil {
			panic(r)
		}
	}()

	// Run all registered systems.
	// This will run the registered init systems if the current tick is 0
	t, err := w.runSystems(ctx, proposal)
	if err != nil {
		span.SetStatus(codes.Error, eris.ToString(err, true))
		span.RecordError(err)
		return nil, err
	}

	if err := w.state.ECB().FinalizeTick(ctx); err != nil {
		span.SetStatus(codes.Error, eris.ToString(err, true))
		span.RecordError(err)
		return nil, err
	}

	return t, nil
}

func (w *World) CommitTick(tick *tick.Tick) error {
	if tick.ID != w.lastFinalizedTickID+1 {
		return eris.New("tick ID must be increment by 1")
	}

	for txHash, receipt := range tick.Receipts {
		receiptBz, err := json.Marshal(receipt)
		if err != nil {
			return eris.Wrap(err, "failed to marshal receipt")
		}
		err = w.receipts.Set(txHash.Bytes(), receiptBz, cacheTTL)
		if err != nil {
			return eris.Wrap(err, "failed to set receipt")
		}
	}

	w.lastFinalizedTickID = tick.ID

	return nil
}
