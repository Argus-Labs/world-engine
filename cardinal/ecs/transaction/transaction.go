package transaction

import (
	"github.com/argus-labs/world-engine/sign"
	"github.com/invopop/jsonschema"
)

type TxMap map[TypeID][]TxAny

type TxAny struct {
	Value any
	ID    TxID
	Sig   *sign.SignedPayload
}

type TxID struct {
	PersonaTag string
	Index      uint64
}

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
