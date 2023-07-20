package transaction

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/invopop/jsonschema"
)

type TypeID int

type ITransaction interface {
	SetID(TypeID) error
	SetEVMType(*abi.Type)
	Name() string
	Schema() *jsonschema.Schema
	ID() TypeID
	Encode(any) ([]byte, error)
	Decode([]byte) (any, error)
	DecodeEVMBytes([]byte) (any, error)
}
