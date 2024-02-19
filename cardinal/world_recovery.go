package cardinal

import (
	"context"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/router/iterator"
	"pkg.world.dev/world-engine/cardinal/worldstage"
)

// recoverAndExecutePendingTxs checks whether the last tick is successfully completed. If not, it will recover
// the pending transactions.
func (w *World) recoverAndExecutePendingTxs() error {
	start, end, err := w.entityStore.GetTickNumbers()
	if err != nil {
		return err
	}
	w.tick.Store(end)
	// We successfully completed the last tick. Everything is fine
	if start == end {
		return nil
	}

	recoveredTxs, err := w.entityStore.Recover(w.msgManager.GetRegisteredMessages())
	if err != nil {
		return err
	}

	// If there is recoevered transactions, we need to reprocess them
	if recoveredTxs != nil {
		w.txQueue = recoveredTxs
		// TODO(scott): this is hacky, but i dont want to fix this now because it's PR scope creep.
		//  but we ideally don't want to treat this as a special tick and should just let it execute normally
		//  from the game loop.
		w.worldStage.CompareAndSwap(worldstage.Starting, worldstage.Running)
		if err = w.Tick(context.Background()); err != nil {
			return err
		}
		w.worldStage.CompareAndSwap(worldstage.Running, worldstage.Starting)
	}

	return nil
}

// RecoverFromChain will attempt to recover the state of the engine based on historical transaction data.
// The function puts the World in a recovery state, and will then query all transaction batches under the World's
// namespace. The function will continuously ask the EVM base shard for batches, and run ticks for each batch returned.
func (w *World) RecoverFromChain(ctx context.Context) error {
	if w.router == nil {
		return eris.Errorf(
			"chain adapter was nil. " +
				"be sure to use the `WithAdapter` option when creating the world",
		)
	}
	if w.CurrentTick() > 0 {
		return eris.Errorf(
			"world recovery should not occur in a world with existing state. please verify all " +
				"state has been cleared before running recovery",
		)
	}

	w.worldStage.CompareAndSwap(worldstage.Starting, worldstage.Recovering)
	defer func() {
		w.worldStage.CompareAndSwap(worldstage.Recovering, worldstage.Ready)
	}()

	err := w.router.TransactionIterator().Each(func(batches []*iterator.TxBatch, tick, timestamp uint64) error {
		if tick < w.CurrentTick() {
			return eris.Errorf("got tick for %d but World was already at %d", tick, w.CurrentTick())
		}
		for w.CurrentTick() != tick {
			if err := w.Tick(ctx); err != nil {
				return eris.Wrap(err, "failed to tick engine")
			}
		}
		for _, batch := range batches {
			w.AddTransaction(batch.MsgID, batch.MsgValue, batch.Tx)
		}
		if err := w.Tick(ctx); err != nil {
			return eris.Wrap(err, "failed to tick engine")
		}
		return nil
	})
	if err != nil {
		return eris.Wrap(err, "encountered error while iterating transactions")
	}
	return nil
}