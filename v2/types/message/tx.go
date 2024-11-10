package message

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/sign"
)

type TxMap = map[string][]Tx

type Tx interface {
	Hash() common.Hash
	Signer() (common.Address, error)
	Verify(common.Address) error
	Namespace() string
	PersonaTag() string
}

type TxType[Msg Message] interface {
	Tx
	Msg() Msg
}

type txType[Msg Message] struct {
	*sign.Transaction
	msg Msg
}

var _ Tx = (*txType[Message])(nil)
var _ TxType[Message] = (*txType[Message])(nil)

func NewTx[Msg Message](tx *sign.Transaction, msg Msg) (Tx, error) {
	if tx == nil {
		return nil, eris.New("transaction is nil")
	}
	return txType[Msg]{
		Transaction: tx,
		msg:         msg,
	}, nil
}

func (t txType[Msg]) Hash() common.Hash {
	return t.Transaction.Hash
}

func (t txType[Msg]) Signer() (common.Address, error) {
	return t.Transaction.Signer()
}

func (t txType[Msg]) Verify(expectedSigner common.Address) error {
	return t.Transaction.Verify(expectedSigner)
}

func (t txType[Msg]) Namespace() string {
	return t.Transaction.Namespace
}

func (t txType[Msg]) PersonaTag() string {
	return t.Transaction.PersonaTag
}

func (t txType[Msg]) Msg() Msg {
	return t.msg
}
