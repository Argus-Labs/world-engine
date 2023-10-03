package public

import (
	"github.com/invopop/jsonschema"
	"pkg.world.dev/world-engine/sign"
)

type ITransaction interface {
	SetID(TransactionTypeID) error
	Name() string
	Schema() (in, out *jsonschema.Schema)
	ID() TransactionTypeID
	Encode(any) ([]byte, error)
	Decode([]byte) (any, error)
	// DecodeEVMBytes decodes ABI encoded bytes into the transactions input type.
	DecodeEVMBytes([]byte) (any, error)
	// ABIEncode encodes the given type in ABI encoding, given that the input is the transaction types input or output
	// type.
	ABIEncode(any) ([]byte, error)
}

type TransactionTypeID int

type TxHash string

type TxAny struct {
	Value  any
	TxHash TxHash
	Sig    *sign.SignedPayload
}
