package transaction

import "github.com/ethereum/go-ethereum/accounts/abi"

type TypeID int

type ITransaction interface {
	SetID(TypeID) error
	SetEVMType(*abi.Type)
	ID() TypeID
	Encode(any) ([]byte, error)
	Decode([]byte) (any, error)
	DecodeEVMBytes([]byte) (any, error)
}
