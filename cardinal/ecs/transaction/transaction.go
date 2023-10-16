package transaction

import (
	"sync"

	"pkg.world.dev/world-engine/sign"
)

type TxQueue struct {
	m          txMap
	txsInQueue int
	mux        *sync.Mutex
}

func NewTxQueue() *TxQueue {
	return &TxQueue{
		m:   txMap{},
		mux: &sync.Mutex{},
	}
}

func (t *TxQueue) GetEVMTxs() []TxAny {
	transactions := make([]TxAny, 0)
	for _, txs := range t.m {
		// skip if theres nothing
		if txs == nil || len(txs) == 0 {
			continue
		}
		for _, tx := range txs {
			if tx.EVMSourceTxHash != "" {
				transactions = append(transactions)
			}
		}
	}
	return transactions
}

func (t *TxQueue) GetAmountOfTxs() int {
	return t.txsInQueue
}

func (t *TxQueue) AddTransaction(id TypeID, v any, sig *sign.SignedPayload) TxHash {
	return t.addTransaction(id, v, sig, "")
}

func (t *TxQueue) AddEVMTransaction(id TypeID, v any, sig *sign.SignedPayload, evmTxHash string) TxHash {
	return t.addTransaction(id, v, sig, evmTxHash)
}

func (t *TxQueue) addTransaction(id TypeID, v any, sig *sign.SignedPayload, evmTxHash string) TxHash {
	t.mux.Lock()
	defer t.mux.Unlock()
	txHash := TxHash(sig.HashHex())
	t.m[id] = append(t.m[id], TxAny{
		TxID:            id,
		TxHash:          txHash,
		Value:           v,
		Sig:             sig,
		EVMSourceTxHash: evmTxHash,
	})
	t.txsInQueue++
	return txHash
}

func (t *TxQueue) CopyTransactions() *TxQueue {
	t.mux.Lock()
	defer t.mux.Unlock()
	cpy := &TxQueue{
		m: t.m,
	}
	t.reset()
	return cpy
}

func (t *TxQueue) reset() {
	t.m = txMap{}
	t.txsInQueue = 0
}

func (t *TxQueue) ForID(id TypeID) []TxAny {
	return t.m[id]
}

type txMap map[TypeID][]TxAny

type TxAny struct {
	TxID   TypeID
	Value  any
	TxHash TxHash
	Sig    *sign.SignedPayload
	// EVMSourceTxHash is the tx hash of the EVM tx that triggered this tx.
	EVMSourceTxHash string
}

type TxHash string

type TypeID int

type ITransaction interface {
	SetID(TypeID) error
	Name() string
	ID() TypeID
	Encode(any) ([]byte, error)
	Decode([]byte) (any, error)
	// DecodeEVMBytes decodes ABI encoded bytes into the transactions input type.
	DecodeEVMBytes([]byte) (any, error)
	// ABIEncode encodes the given type in ABI encoding, given that the input is the transaction types input or output
	// type.
	ABIEncode(any) ([]byte, error)
}
