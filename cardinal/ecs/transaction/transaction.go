package transaction

import (
	"github.com/invopop/jsonschema"
)

type TypeID int

type ITransaction interface {
	SetID(TypeID) error
	Name() string
	Schema() *jsonschema.Schema
	ID() TypeID
	Encode(any) ([]byte, error)
	Decode([]byte) (any, error)
	DecodeEVMBytes([]byte) (any, error)
	ABIEncode(any) ([]byte, error)
}
