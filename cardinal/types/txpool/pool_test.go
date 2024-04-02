package txpool

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/storage"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/sign"
)

var _ storage.NonceValidator = &noEvenNonceValidator{}

// noEvenNonceValidator will reject any even nonce transactions.
type noEvenNonceValidator struct {
	enableTest bool
}

func (f *noEvenNonceValidator) IsNonceValid(signerAddress string, nonce uint64) error {
	if f.enableTest {
		if nonce%2 == 0 {
			return errors.New("some error")
		}
		return nil
	}
	return nil
}

// TestPool_CleanPool tests that when
func TestPool_CleanPool(t *testing.T) {
	dummyHash := common.HexToHash("0x4d74864c5783f0960478b15a4e2978ea8ac2ae4b088758f0716401edd9d07abf")
	dummySig := "bf57d31df75bbf5b3a61d9b2d51617b7f3cfff7ecd71a034c6357234f53c9def553a817f7cbc8c823d71b1a9dd2398ed97591d5da9c610b4a1d28e87aa62f85d00"
	nonceValidator := &noEvenNonceValidator{}
	type txsWithID struct {
		id  types.MessageID
		txs []*sign.Transaction
	}
	txp := New(WithNonceValidator(nonceValidator))
	txs2left := txsWithID{
		id: types.MessageID(1),
		txs: []*sign.Transaction{
			{
				Nonce:     1,
				Hash:      dummyHash,
				Signature: dummySig,
			},
			{
				Nonce:     2,
				Hash:      dummyHash,
				Signature: dummySig,
			},
			{
				Nonce:     3,
				Hash:      dummyHash,
				Signature: dummySig,
			},
		},
	}
	allGoneTxs := txsWithID{
		id: types.MessageID(2),
		txs: []*sign.Transaction{
			{
				Nonce:     2,
				Hash:      dummyHash,
				Signature: dummySig,
			},
			{
				Nonce:     4,
				Hash:      dummyHash,
				Signature: dummySig,
			},
			{
				Nonce:     6,
				Hash:      dummyHash,
				Signature: dummySig,
			},
		},
	}
	txsAll3 := txsWithID{
		id: types.MessageID(3),
		txs: []*sign.Transaction{
			{
				Nonce:     3,
				Hash:      dummyHash,
				Signature: dummySig,
			},
			{
				Nonce:     5,
				Hash:      dummyHash,
				Signature: dummySig,
			},
			{
				Nonce:     9,
				Hash:      dummyHash,
				Signature: dummySig,
			},
		},
	}

	allTxs := []txsWithID{txs2left, allGoneTxs, txsAll3}
	for _, txGroup := range allTxs {
		for _, tx := range txGroup.txs {
			_, err := txp.AddTransaction(txGroup.id, nil, tx)
			assert.NilError(t, err)
		}
	}

	nonceValidator.enableTest = true
	txp.CleanPool()

	txs := txp.Transactions()
	assert.Len(t, txs[allGoneTxs.id], 0)
	assert.Len(t, txs[txs2left.id], 2)
	assert.Len(t, txs[txsAll3.id], 3)
}
