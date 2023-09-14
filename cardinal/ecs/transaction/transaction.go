package transaction

import (
	"sync"

	"github.com/invopop/jsonschema"
	"pkg.world.dev/world-engine/sign"
)

type TxQueue struct {
	m   txMap
	mux *sync.Mutex
}

func NewTxQueue() *TxQueue {
	return &TxQueue{
		m:   txMap{},
		mux: &sync.Mutex{},
	}
}

func (t *TxQueue) AddTransaction(id TypeID, v any, sig *sign.SignedPayload) TxHash {
	t.mux.Lock()
	defer t.mux.Unlock()
	txHash := TxHash(sig.HashHex())
	t.m[id] = append(t.m[id], TxAny{
		TxHash: txHash,
		Value:  v,
		Sig:    sig,
	})
	return txHash
}

func (t *TxQueue) CopyTransaction() *TxQueue {
	t.mux.Lock()
	defer t.mux.Unlock()
	cpy := &TxQueue{
		m: t.m,
	}
	t.m = txMap{}
	return cpy
}

func (t *TxQueue) ForID(id TypeID) []TxAny {
	return t.m[id]
}

type txMap map[TypeID][]TxAny

type TxAny struct {
	Value  any
	TxHash TxHash
	Sig    *sign.SignedPayload
}

type TxHash string

type TypeID int

type ITransaction interface {
	SetID(TypeID) error
	Name() string
	Schema() (in, out *jsonschema.Schema)
	ID() TypeID
	Encode(any) ([]byte, error)
	Decode([]byte) (any, error)
	// DecodeEVMBytes decodes ABI encoded bytes into the transactions input type.
	DecodeEVMBytes([]byte) (any, error)
	// ABIEncode encodes the given type in ABI encoding, given that the input is the transaction types input or output
	// type.
	ABIEncode(any) ([]byte, error)
}
