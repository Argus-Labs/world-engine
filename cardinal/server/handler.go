package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/sign"
)

func (t *Handler) processTransaction(tx transaction.ITransaction, payload []byte, sp *sign.SignedPayload) ([]byte, error) {
	txVal, err := tx.Decode(payload)
	if err != nil {
		return nil, fmt.Errorf("unable to decode transaction: %w", err)
	}

	submitTx := func() (uint64, []byte, error) {
		tick, txHash := t.w.AddTransaction(tx.ID(), txVal, sp)

		res, err := json.Marshal(TransactionReply{
			TxHash: string(txHash),
			Tick:   tick,
		})
		if err != nil {
			return 0, nil, fmt.Errorf("unable to marshal response: %w", err)
		}
		return tick, res, nil
	}

	// check if we have an adapter
	if t.adapter != nil {
		// if the world is recovering via adapter, we shouldn't accept transactions.
		if t.w.IsRecovering() {
			return nil, errors.New("unable to submit transactions: game world is recovering state")
		} else {
			tick, res, err := submitTx()
			if err != nil {
				return nil, err
			}
			err = t.adapter.Submit(context.Background(), sp, uint64(tx.ID()), tick)
			if err != nil {
				return nil, fmt.Errorf("error submitting transaction to blockchain: %w", err)
			}
			return res, nil
		}
	} else {
		// if there is no adapter, then we can just put the tx in the queue.
		_, res, err := submitTx()
		if err != nil {
			return nil, err
		}
		return res, nil
	}
}
