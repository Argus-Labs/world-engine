package cardinal

import (
	"context"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
	"pkg.world.dev/world-engine/cardinal/txpool"
	"pkg.world.dev/world-engine/cardinal/types"
	shardTypes "pkg.world.dev/world-engine/evm/x/shard/types"
	shardv1 "pkg.world.dev/world-engine/rift/shard/v1"
	"pkg.world.dev/world-engine/sign"
)

// recoverGameState checks the status of the last game tick. If the tick was incomplete (indicating
// a problem when running one of the Systems), the snapshotted state is recovered and the pending
// transactions for the incomplete tick are returned. A nil recoveredTxs indicates there are no pending
// transactions that need to be processed because the last tick was successful.
func (w *World) recoverGameState() (recoveredTxs *txpool.TxQueue, err error) {
	start, end, err := w.entityStore.GetTickNumbers()
	if err != nil {
		return nil, err
	}
	w.tick.Store(end)
	// We successfully completed the last tick. Everything is fine
	if start == end {
		//nolint:nilnil // its ok.
		return nil, nil
	}
	return w.entityStore.Recover(w.msgManager.GetRegisteredMessages())
}

// RecoverFromChain will attempt to recover the state of the engine based on historical transaction data.
// The function puts the engine in a recovery state, and then queries all transaction batches under the engine's
// namespace. The function will continuously ask the EVM base shard for batches, and run ticks for each batch returned.
//
//nolint:gocognit
func (w *World) RecoverFromChain(ctx context.Context) error {
	if w.chain == nil {
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

	w.isRecovering.Store(true)
	defer func() {
		w.isRecovering.Store(false)
	}()
	namespace := w.namespace.String()
	var nextKey []byte
	for {
		res, err := w.chain.QueryTransactions(
			ctx, &shardTypes.QueryTransactionsRequest{
				Namespace: namespace,
				Page: &shardTypes.PageRequest{
					Key: nextKey,
				},
			},
		)
		if err != nil {
			return err
		}
		for _, tickedTxs := range res.Epochs {
			target := tickedTxs.Epoch
			// tick up to target
			if target < w.CurrentTick() {
				return eris.Errorf(
					"got tx for tick %d, but world is at tick %d",
					target,
					w.CurrentTick(),
				)
			}
			for current := w.CurrentTick(); current != target; {
				if err = w.Tick(ctx); err != nil {
					return err
				}
				current = w.CurrentTick()
			}
			// we've now reached target. we need to inject the transactions and tick.
			transactions := tickedTxs.Txs
			for _, tx := range transactions {
				sp, err := w.decodeTransaction(tx.GameShardTransaction)
				if err != nil {
					return err
				}
				msg := w.msgManager.GetMessage(types.MessageID(tx.TxId))
				if msg == nil {
					return eris.Errorf("error recovering tx with EntityID %d: tx id not found", tx.TxId)
				}
				v, err := msg.Decode(sp.Body)
				if err != nil {
					return err
				}
				w.AddTransaction(types.MessageID(tx.TxId), v, w.protoTransactionToGo(sp))
			}
			// run the tick for this batch
			if err = w.Tick(ctx); err != nil {
				return err
			}
		}

		// if a page response was in the reply, that means there is more data to read.
		if res.Page != nil {
			// case where the next key is empty or nil, we don't want to continue the queries.
			if res.Page.Key == nil || len(res.Page.Key) == 0 {
				break
			}
			nextKey = res.Page.Key
		} else {
			// if the entire page reply is nil, then we are definitely done.
			break
		}
	}
	return nil
}

func (w *World) protoTransactionToGo(sp *shardv1.Transaction) *sign.Transaction {
	return &sign.Transaction{
		PersonaTag: sp.PersonaTag,
		Namespace:  sp.Namespace,
		Nonce:      sp.Nonce,
		Signature:  sp.Signature,
		Body:       sp.Body,
	}
}

func (w *World) decodeTransaction(bz []byte) (*shardv1.Transaction, error) {
	payload := new(shardv1.Transaction)
	err := proto.Unmarshal(bz, payload)
	return payload, eris.Wrap(err, "")
}
