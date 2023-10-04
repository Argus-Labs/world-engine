package transaction

import (
	"errors"
	"fmt"
	"reflect"
	"sync"

	ethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"pkg.world.dev/world-engine/cardinal/ecs/abi"
	"pkg.world.dev/world-engine/cardinal/ecs/codec"
	"pkg.world.dev/world-engine/cardinal/ecs/itransaction"
	"pkg.world.dev/world-engine/cardinal/ecs/receipt"
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

func (t *TxQueue) GetAmountOfTxs() int {
	t.mux.Lock()
	defer t.mux.Unlock()
	acc := 0
	for _, v := range t.m {
		acc += len(v)
	}
	return acc
}

func (t *TxQueue) AddTransaction(id itransaction.TypeID, v any, sig *sign.SignedPayload) itransaction.TxHash {
	t.mux.Lock()
	defer t.mux.Unlock()
	txHash := itransaction.TxHash(sig.HashHex())
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

func (t *TxQueue) ForID(id itransaction.TypeID) []TxAny {
	return t.m[id]
}

type txMap map[itransaction.TypeID][]TxAny

type TxAny struct {
	Value  any
	TxHash itransaction.TxHash
	Sig    *sign.SignedPayload
}

var (
	ErrEVMTypeNotSet = errors.New("EVM type is not set")
)

var _ itransaction.ITransaction = NewTransactionType[struct{}, struct{}]("")

// TransactionType helps manage adding transactions (aka events) to the world transaction queue. It also assists
// in the using of transactions inside of System functions.
type TransactionType[In, Out any] struct {
	id         itransaction.TypeID
	isIDSet    bool
	name       string
	inEVMType  *ethabi.Type
	outEVMType *ethabi.Type
}

func WithTxEVMSupport[In, Out any]() func(transactionType *TransactionType[In, Out]) {
	return func(txt *TransactionType[In, Out]) {
		var in In
		abiType, err := abi.GenerateABIType(in)
		if err != nil {
			panic(err)
		}
		txt.inEVMType = abiType

		var out Out
		abiType, err = abi.GenerateABIType(out)
		if err != nil {
			panic(err)
		}
		txt.outEVMType = abiType
	}
}

func NewTransactionType[In, Out any](
	name string,
	opts ...func() func(*TransactionType[In, Out]),
) *TransactionType[In, Out] {

	var in In
	var out Out
	inType := reflect.TypeOf(in)
	inKind := inType.Kind()
	inValid := false
	if (inKind == reflect.Pointer && inType.Elem().Kind() == reflect.Struct) || inKind == reflect.Struct {
		inValid = true
	}
	outType := reflect.TypeOf(out)
	outKind := inType.Kind()
	outValid := false
	if (outKind == reflect.Pointer && outType.Elem().Kind() == reflect.Struct) || outKind == reflect.Struct {
		outValid = true
	}

	if !outValid || !inValid {
		panic(fmt.Sprintf("Invalid TransactionType: %s: The In and Out must be both structs", name))
	}

	txt := &TransactionType[In, Out]{
		name: name,
	}
	for _, opt := range opts {
		opt()(txt)
	}
	return txt
}

func (t *TransactionType[In, Out]) Name() string {
	return t.name
}

func (t *TransactionType[In, Out]) ID() itransaction.TypeID {
	if !t.isIDSet {
		panic(fmt.Sprintf("id on %v is not set", t))
	}
	return t.id
}

var emptySignature = &sign.SignedPayload{}

// AddToQueue adds a transaction with the given data to the world object. The transaction will be executed
// at the next game tick. An optional sign.SignedPayload can be associated with this transaction.
func (t *TransactionType[In, Out]) AddToQueue(txQueue *TxQueue, data In, sigs ...*sign.SignedPayload) itransaction.TxHash {
	sig := emptySignature
	if len(sigs) > 0 {
		sig = sigs[0]
	}
	id := txQueue.AddTransaction(t.ID(), data, sig)
	//_, id := world.AddTransaction(t.ID(), data, sig)
	return id
}

func (t *TransactionType[In, Out]) SetID(id itransaction.TypeID) error {
	if t.isIDSet {
		// In games implemented with Cardinal, transactions will only be initialized one time (on startup).
		// In tests, it's often useful to use the same transaction in multiple worlds. This check will allow for the
		// re-initialization of transactions, as long as the ID doesn't change.
		if id == t.id {
			return nil
		}
		return fmt.Errorf("id on transaction %v is already set to %v and cannot change to %d", t, t.id, id)
	}
	t.id = id
	t.isIDSet = true
	return nil
}

type TxData[In any] struct {
	TxHash itransaction.TxHash
	Value  In
	Sig    *sign.SignedPayload
}

func (t *TransactionType[In, Out]) AddError(receiptHistory *receipt.History, hash itransaction.TxHash, err error) {
	receiptHistory.AddError(hash, err)
}

func (t *TransactionType[In, Out]) SetResult(receiptHistory *receipt.History, hash itransaction.TxHash, result Out) {
	receiptHistory.SetResult(hash, result)
}

func (t *TransactionType[In, Out]) GetReceipt(receiptHistory *receipt.History, hash itransaction.TxHash) (v Out, errs []error, ok bool) {
	getReceipt := func() (any, []error, bool) {
		rec, ok := receiptHistory.GetReceipt(hash)
		if !ok {
			return nil, nil, false
		}
		return rec.Result, rec.Errs, true
	}
	iface, errs, ok := getReceipt()
	if !ok {
		return v, nil, false
	}
	// if iface is nil, maybe the result has never been set. The errors may still be valid.
	if iface == nil {
		return v, errs, true
	}
	value, ok := iface.(Out)
	if !ok {
		return v, nil, false
	}
	return value, errs, true
}

// In extracts all the transactions in the transaction queue that match this TransactionType's ID.
func (t *TransactionType[In, Out]) In(tq *TxQueue) []TxData[In] {
	var txs []TxData[In]
	for _, tx := range tq.ForID(t.ID()) {
		if val, ok := tx.Value.(In); ok {
			txs = append(txs, TxData[In]{
				TxHash: tx.TxHash,
				Value:  val,
				Sig:    tx.Sig,
			})
		}
	}
	return txs
}

func (t *TransactionType[In, Out]) Encode(a any) ([]byte, error) {
	return codec.Encode(a)
}

func (t *TransactionType[In, Out]) Decode(bytes []byte) (any, error) {
	return codec.Decode[In](bytes)
}

// ABIEncode encodes the input to the transactions matching evm type. If the input is not either of the transactions
// evm types, an error is returned.
func (t *TransactionType[In, Out]) ABIEncode(v any) ([]byte, error) {
	if t.inEVMType == nil || t.outEVMType == nil {
		return nil, ErrEVMTypeNotSet
	}

	var args ethabi.Arguments
	var input any
	switch v.(type) {
	case In:
		input = v.(In)
		args = ethabi.Arguments{{Type: *t.inEVMType}}
	case Out:
		input = v.(Out)
		args = ethabi.Arguments{{Type: *t.outEVMType}}
	default:
		return nil, fmt.Errorf("expected input to be of type %T or %T, got %T", new(In), new(Out), v)
	}

	return args.Pack(input)
}

// DecodeEVMBytes decodes abi encoded solidity structs into the transaction's "In" type.
func (t *TransactionType[In, Out]) DecodeEVMBytes(bz []byte) (any, error) {
	if t.inEVMType == nil {
		return nil, ErrEVMTypeNotSet
	}
	args := ethabi.Arguments{{Type: *t.inEVMType}}
	unpacked, err := args.Unpack(bz)
	if err != nil {
		return nil, err
	}
	if len(unpacked) < 1 {
		return nil, fmt.Errorf("error decoding EVM bytes: no values could be unpacked into the abi type")
	}
	input, err := abi.SerdeInto[In](unpacked[0])
	if err != nil {
		return nil, err
	}
	return input, nil
}
