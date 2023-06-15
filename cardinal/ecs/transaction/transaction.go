package transaction

type TypeID int

type ITransaction interface {
	SetID(TypeID) error
	ID() TypeID
	Name() string
	Encode(any) ([]byte, error)
	Decode([]byte) (any, error)
}
