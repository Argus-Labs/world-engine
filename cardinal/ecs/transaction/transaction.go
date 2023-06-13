package transaction

type TypeID int

type ITransaction interface {
	SetID(TypeID) error
	ID() TypeID
}
