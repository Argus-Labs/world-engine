package component

type (
	TypeID int

	// IComponentType is a high level representation of a user defined component struct.
	IComponentType interface {
		// ID returns the ID of the component.
		ID() TypeID
		// New creates a new pointer to the component struct. It will set the struct being pointed to with the default
		//value if the component was created with one. Otherwise, the struct being pointed to will be empty.
		New() ([]byte, error)
		// Name returns the name of the component.
		Name() string
	}
)
