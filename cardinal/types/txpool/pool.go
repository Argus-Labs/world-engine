package txpool

import (
	"sync"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/storage"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/sign"
)

type TxMap map[types.MessageID][]TxData

type TxData struct {
	MsgID  types.MessageID
	Msg    any
	TxHash types.TxHash
	Tx     *sign.Transaction
	// EVMSourceTxHash is the tx hash of the EVM tx that triggered this tx. This field will only be populated for
	// transactions received from the EVM via Router.
	EVMSourceTxHash string
}

type TxPool struct {
	m          TxMap
	txsInPool  int
	mux        *sync.Mutex
	nonceStore storage.NonceValidator
}

type Option func(*TxPool)

// WithNonceValidator enables nonce validation for the txpool. If this option is used, the pool will only accept
// transactions with a valid nonce value. Do note, however, that there is still a chance that, even though a transaction
// might be accepted to the pool with a valid nonce, it could still be failed later; this is because a transaction may
// be appended to the pool while a tick is still processing transactions where one of the transactions has the same
// nonce.
func WithNonceValidator(ns storage.NonceValidator) Option {
	return func(pool *TxPool) {
		pool.nonceStore = ns
	}
}

func New(opts ...Option) *TxPool {
	txp := &TxPool{
		m:   TxMap{},
		mux: &sync.Mutex{},
	}
	for _, opt := range opts {
		opt(txp)
	}
	return txp
}

func (t *TxPool) GetAmountOfTxs() int {
	return t.txsInPool
}

// GetEVMTxs gets all the txs in the queue that originated from the EVM.
// NOTE: this is called ONLY in the copied tx queue in world.doTick, so we do not need to use the mutex here.
func (t *TxPool) GetEVMTxs() []TxData {
	transactions := make([]TxData, 0)
	for _, txs := range t.m {
		// skip if theres nothing
		if len(txs) == 0 {
			continue
		}
		for _, tx := range txs {
			if tx.EVMSourceTxHash != "" {
				transactions = append(transactions, tx)
			}
		}
	}
	return transactions
}

// AddTransaction adds a transaction to the pool, returning the tx hash. Note, an error will only be returned if this
// txpool was instantiated with the WithNonceValidator option.
func (t *TxPool) AddTransaction(id types.MessageID, v any, sig *sign.Transaction) (types.TxHash, error) {
	if t.nonceStore != nil {
		addr, err := sig.PubKey()
		if err != nil {
			return "", eris.Wrap(err, "failed to get PubKey from transaction")
		}
		if err := t.nonceStore.IsNonceValid(addr, sig.Nonce); err != nil {
			return "", eris.Wrap(err, "failed to validate nonce")
		}
	}
	return t.addTransaction(id, v, sig, ""), nil
}

func (t *TxPool) AddEVMTransaction(id types.MessageID, v any, sig *sign.Transaction, evmTxHash string) types.TxHash {
	return t.addTransaction(id, v, sig, evmTxHash)
}

func (t *TxPool) addTransaction(id types.MessageID, v any, sig *sign.Transaction, evmTxHash string) types.TxHash {
	t.mux.Lock()
	defer t.mux.Unlock()
	txHash := types.TxHash(sig.HashHex())
	t.m[id] = append(t.m[id], TxData{
		MsgID:           id,
		TxHash:          txHash,
		Msg:             v,
		Tx:              sig,
		EVMSourceTxHash: evmTxHash,
	})
	t.txsInPool++
	return txHash
}

func (t *TxPool) Transactions() TxMap {
	return t.m
}

// CopyTransactions returns a copy of the TxPool, and resets the state to 0 values.
func (t *TxPool) CopyTransactions() *TxPool {
	t.mux.Lock()
	defer t.mux.Unlock()
	cpy := *t
	t.reset()
	return &cpy
}

func (t *TxPool) reset() {
	t.m = TxMap{}
	t.txsInPool = 0
}

func (t *TxPool) ForID(id types.MessageID) []TxData {
	return t.m[id]
}

func (t *TxPool) CleanPool() {
	t.mux.Lock()
	defer t.mux.Unlock()
	for txTypeID, txDatas := range t.m {
		indexesToDelete := make([]int, 0)
		for i, txData := range txDatas {
			pk, err := txData.Tx.PubKey()
			if err != nil {
				indexesToDelete = append(indexesToDelete, i)
			} else {
				if err = t.nonceStore.IsNonceValid(pk, txData.Tx.Nonce); err != nil {
					indexesToDelete = append(indexesToDelete, i)
				}
			}
		}
		// Remove transactions in reverse order to maintain the correct indices
		for i := len(indexesToDelete) - 1; i >= 0; i-- {
			indexToDelete := indexesToDelete[i]
			txDatas = append(txDatas[:indexToDelete], txDatas[indexToDelete+1:]...)
		}
		t.m[txTypeID] = txDatas
	}
}
