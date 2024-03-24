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
	log.Debug().Msg("Checking to see if last tick was successfully completed")
	start, end, err := w.entityStore.GetTickNumbers()
	if err != nil {
		return err
	}
	w.tick.Store(end)
	w.receiptHistory.SetTick(end)

	// We successfully completed the last tick. Everything is fine
	if start == end {
		log.Debug().Msg("No pending transactions to recover")
		return nil
	}

	log.Debug().Msg("Last tick was not successfully completed, checking to see if there are pending transactions")

	// Read from redis to see if there are any pending transactions
	recoveredTxs, err := w.entityStore.Recover(w.msgManager.GetRegisteredMessages())
	if err != nil {
		return err
	}

	// If there is recovered transactions, we need to reprocess them
	if recoveredTxs != nil {
		log.Debug().Msg("Recovered transactions found, reprocessing")
		w.txPool = recoveredTxs
		// TODO(scott): this is hacky, but i dont want to fix this now because it's PR scope creep.
		//  but we ideally don't want to treat this as a special tick and should just let it execute normally
		//  from the game loop.
		if err = w.doTick(context.Background(), uint64(time.Now().Unix())); err != nil {
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
		log.Info().Msg("Chain router is not set, skipping chain recovery")
		return nil
	}
	log.Info().Msg("Recovering from chain")

	start := w.CurrentTick()
	err := w.router.TransactionIterator().Each(func(batches []*iterator.TxBatch, tick, timestamp uint64) error {
		for w.CurrentTick() != tick {
			if err := w.doTick(ctx, timestamp); err != nil {
				return eris.Wrap(err, "failed to tick engine")
			}
		}

		for _, batch := range batches {
			w.AddTransaction(batch.MsgID, batch.MsgValue, batch.Tx)
		}

		if err := w.doTick(ctx, timestamp); err != nil {
			return eris.Wrap(err, "failed to tick engine")
		}
		return nil
	}, start)
	if err != nil {
		return eris.Wrap(err, "encountered error while iterating transactions")
	}
	return nil
}
