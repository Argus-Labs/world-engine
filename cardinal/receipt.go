package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/receipt"
	"pkg.world.dev/world-engine/cardinal/txpool"
	"pkg.world.dev/world-engine/cardinal/types/message"
)

type EVMTxReceipt struct {
	ABIResult []byte
	Errs      []error
	EVMTxHash string
}

func (w *World) AddMessageError(id message.TxHash, err error) {
	w.receiptHistory.AddError(id, err)
}

func (w *World) SetMessageResult(id message.TxHash, a any) {
	w.receiptHistory.SetResult(id, a)
}

func (w *World) GetTransactionReceipt(id message.TxHash) (any, []error, bool) {
	rec, ok := w.receiptHistory.GetReceipt(id)
	if !ok {
		return nil, nil, false
	}
	return rec.Result, rec.Errs, true
}

func (w *World) GetTransactionReceiptsForTick(tick uint64) ([]receipt.Receipt, error) {
	return w.receiptHistory.GetReceiptsForTick(tick)
}

// ConsumeEVMMsgResult consumes a tx result from an EVM originated Cardinal message.
// It will fetch the receipt from the map, and then delete ('consume') it from the map.
func (w *World) ConsumeEVMMsgResult(evmTxHash string) (EVMTxReceipt, bool) {
	r, ok := w.evmTxReceipts[evmTxHash]
	delete(w.evmTxReceipts, evmTxHash)
	return r, ok
}

func (w *World) ReceiptHistorySize() uint64 {
	return w.receiptHistory.Size()
}

func (w *World) setEvmResults(txs []txpool.TxData) {
	// iterate over all EVM originated transactions
	for _, tx := range txs {
		// see if tx has a receipt. sometimes it won't because:
		// The system isn't using TxIterators && never explicitly called SetResult.
		rec, ok := w.receiptHistory.GetReceipt(tx.TxHash)
		if !ok {
			continue
		}
		evmRec := EVMTxReceipt{EVMTxHash: tx.EVMSourceTxHash}
		msg := w.msgManager.GetMessage(tx.MsgID)
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
		w.evmTxReceipts[evmRec.EVMTxHash] = evmRec
	}
}
