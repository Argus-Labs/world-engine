package transaction

import (
	"github.com/argus-labs/world-engine/sign"
	"github.com/ethereum/go-ethereum/accounts/abi"
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
	SetEVMType(*abi.Type)
	Name() string
	Schema() (in, out *jsonschema.Schema)
	ID() TypeID
	Encode(any) ([]byte, error)
	Decode([]byte) (any, error)
	DecodeEVMBytes([]byte) (any, error)
}
