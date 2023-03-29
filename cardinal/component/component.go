package component

import (
	"unsafe"
)

type (
	TypeID int

	IComponentType interface {
		ID() TypeID
		New() unsafe.Pointer
		Name() string
	}
)
