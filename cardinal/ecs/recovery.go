package ecs

import (
	"context"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
	"pkg.world.dev/world-engine/cardinal/txpool"
	"pkg.world.dev/world-engine/cardinal/types/message"
	"pkg.world.dev/world-engine/evm/x/shard/types"
	shardv1 "pkg.world.dev/world-engine/rift/shard/v1"
	"pkg.world.dev/world-engine/sign"
)

// recoverGameState checks the status of the last game tick. If the tick was incomplete (indicating
// a problem when running one of the Systems), the snapshotted state is recovered and the pending
// transactions for the incomplete tick are returned. A nil recoveredTxs indicates there are no pending
// transactions that need to be processed because the last tick was successful.
func (e *Engine) recoverGameState() (recoveredTxs *txpool.TxQueue, err error) {
	start, end, err := e.TickStore().GetTickNumbers()
	if err != nil {
		return nil, err
	}
	e.tick.Store(end)
	// We successfully completed the last tick. Everything is fine
	if start == end {
		//nolint:nilnil // its ok.
		return nil, nil
	}
	return e.TickStore().Recover(e.msgManager.GetRegisteredMessages())
}

// RecoverFromChain will attempt to recover the state of the engine based on historical transaction data.
// The function puts the engine in a recovery state, and then queries all transaction batches under the engine's
// namespace. The function will continuously ask the EVM base shard for batches, and run ticks for each batch returned.
//
//nolint:gocognit
func (e *Engine) RecoverFromChain(ctx context.Context) error {
	if e.chain == nil {
		return eris.Errorf(
			"chain adapter was nil. " +
				"be sure to use the `WithAdapter` option when creating the world",
		)
	}
	if e.CurrentTick() > 0 {
		return eris.Errorf(
			"world recovery should not occur in a world with existing state. please verify all " +
				"state has been cleared before running recovery",
		)
	}

	e.isRecovering.Store(true)
	defer func() {
		e.isRecovering.Store(false)
	}()
	namespace := e.Namespace().String()
	var nextKey []byte
	for {
		res, err := e.chain.QueryTransactions(
			ctx, &types.QueryTransactionsRequest{
				Namespace: namespace,
				Page: &types.PageRequest{
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
			if target < e.CurrentTick() {
				return eris.Errorf(
					"got tx for tick %d, but world is at tick %d",
					target,
					e.CurrentTick(),
				)
			}
			for current := e.CurrentTick(); current != target; {
				if err = e.Tick(ctx); err != nil {
					return err
				}
				current = e.CurrentTick()
			}
			// we've now reached target. we need to inject the transactions and tick.
			transactions := tickedTxs.Txs
			for _, tx := range transactions {
				sp, err := e.decodeTransaction(tx.GameShardTransaction)
				if err != nil {
					return err
				}
				msg := e.msgManager.GetMessage(message.TypeID(tx.TxId))
				if msg == nil {
					return eris.Errorf("error recovering tx with ID %d: tx id not found", tx.TxId)
				}
				v, err := msg.Decode(sp.Body)
				if err != nil {
					return err
				}
				e.AddTransaction(message.TypeID(tx.TxId), v, e.protoTransactionToGo(sp))
			}
			// run the tick for this batch
			if err = e.Tick(ctx); err != nil {
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

func (e *Engine) protoTransactionToGo(sp *shardv1.Transaction) *sign.Transaction {
	return &sign.Transaction{
		PersonaTag: sp.PersonaTag,
		Namespace:  sp.Namespace,
		Nonce:      sp.Nonce,
		Signature:  sp.Signature,
		Body:       sp.Body,
	}
}

func (e *Engine) decodeTransaction(bz []byte) (*shardv1.Transaction, error) {
	payload := new(shardv1.Transaction)
	err := proto.Unmarshal(bz, payload)
	return payload, eris.Wrap(err, "")
}
