package component

type (
	TypeID int

	// IComponentType is a high level representation of a user defined component struct.
	IComponentType interface {
		// ID returns the ID of the component.
		ID() TypeID
		// New returns the marshaled bytes of the default value for the component struct.
		New() ([]byte, error)
		// Name returns the name of the component.
		Name() string
	}
)
