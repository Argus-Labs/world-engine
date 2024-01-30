package ecs

import (
	"pkg.world.dev/world-engine/cardinal/ecs/receipt"
	"pkg.world.dev/world-engine/cardinal/txpool"
	"pkg.world.dev/world-engine/cardinal/types/message"
)

type EVMTxReceipt struct {
	ABIResult []byte
	Errs      []error
	EVMTxHash string
}

func (e *Engine) AddMessageError(id message.TxHash, err error) {
	e.receiptHistory.AddError(id, err)
}

func (e *Engine) SetMessageResult(id message.TxHash, a any) {
	e.receiptHistory.SetResult(id, a)
}

func (e *Engine) GetTransactionReceipt(id message.TxHash) (any, []error, bool) {
	rec, ok := e.receiptHistory.GetReceipt(id)
	if !ok {
		return nil, nil, false
	}
	return rec.Result, rec.Errs, true
}

func (e *Engine) GetTransactionReceiptsForTick(tick uint64) ([]receipt.Receipt, error) {
	return e.receiptHistory.GetReceiptsForTick(tick)
}

// ConsumeEVMMsgResult consumes a tx result from an EVM originated Cardinal message.
// It will fetch the receipt from the map, and then delete ('consume') it from the map.
func (e *Engine) ConsumeEVMMsgResult(evmTxHash string) (EVMTxReceipt, bool) {
	r, ok := e.evmTxReceipts[evmTxHash]
	delete(e.evmTxReceipts, evmTxHash)
	return r, ok
}

func (e *Engine) ReceiptHistorySize() uint64 {
	return e.receiptHistory.Size()
}

func (e *Engine) setEvmResults(txs []txpool.TxData) {
	// iterate over all EVM originated transactions
	for _, tx := range txs {
		// see if tx has a receipt. sometimes it won't because:
		// The system isn't using TxIterators && never explicitly called SetResult.
		rec, ok := e.receiptHistory.GetReceipt(tx.TxHash)
		if !ok {
			continue
		}
		evmRec := EVMTxReceipt{EVMTxHash: tx.EVMSourceTxHash}
		msg := e.msgManager.GetMessage(tx.MsgID)
		if rec.Result != nil {
			abiBz, err := msg.ABIEncode(rec.Result)
			if err != nil {
				rec.Errs = append(rec.Errs, err)
			}
			evmRec.ABIResult = abiBz
		}
		if len(rec.Errs) > 0 {
			evmRec.Errs = rec.Errs
		}
		e.evmTxReceipts[evmRec.EVMTxHash] = evmRec
	}
}
