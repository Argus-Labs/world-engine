package cardinal

import (
	"context"
	"time"

	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"

	"pkg.world.dev/world-engine/cardinal/router/iterator"
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

	recoveredTxs, err := w.entityStore.Recover(w.GetRegisteredMessages())
	if err != nil {
		return err
	}

	// If there is recovered transactions, we need to reprocess them
	if recoveredTxs != nil {
		w.txPool = recoveredTxs
		// TODO(scott): this is hacky, but i dont want to fix this now because it's PR scope creep.
		//  but we ideally don't want to treat this as a special tick and should just let it execute normally
		//  from the game loop.
		if err = w.doTick(context.Background(), uint64(time.Now().UnixNano())); err != nil {
			return err
		}
	}

	return nil
}

// RecoverFromChain will attempt to recover the state of the engine based on historical transaction data.
// The function puts the World in a recovery state, and will then query all transaction batches under the World's
// namespace. The function will continuously ask the EVM base shard for batches, and run ticks for each batch returned.
func (w *World) RecoverFromChain(ctx context.Context) error {
	if w.router == nil {
		return eris.Errorf(
			"chain router was nil. " +
				"be sure to use the `WithAdapter` option when creating the world",
		)
	}

	log.Info().Msgf("Synchronizing state from base shard starting from tick %d", w.CurrentTick())

	start := w.CurrentTick()
	err := w.router.TransactionIterator().Each(func(batches []*iterator.TxBatch, tick, timestamp uint64) error {
		log.Info().Msgf("Found transactions for tick %d", tick)

		if w.CurrentTick() != tick {
			log.Info().Msgf("Fast forwarding to tick %d from %d", tick, w.CurrentTick())
		}
		for w.CurrentTick() != tick {
			if err := w.doTick(ctx, timestamp); err != nil {
				return eris.Wrap(err, "failed to tick world")
			}
		}
		log.Info().Msgf("Successfully fast forwarded to tick %d", tick)

		for _, batch := range batches {
			w.AddTransaction(batch.MsgID, batch.MsgValue, batch.Tx)
		}

		log.Info().Msgf("Executing tick %d in recovery mode", tick)
		if err := w.doTick(ctx, timestamp); err != nil {
			return eris.Wrap(err, "failed to tick world")
		}
		return nil
	}, start)
	if err != nil {
		return eris.Wrap(err, "encountered error while iterating transactions")
	}

	log.Info().Msgf("Successfully synchronized state from base shard")

	return nil
}
