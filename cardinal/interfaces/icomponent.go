package interfaces

// IComponentType is a high level representation of a user defined component struct.
type IComponentType interface {
	// SetID sets the ID of this component. It must only be set once
	SetID(ComponentTypeID) error
	// ID returns the ID of the component.
	ID() ComponentTypeID
	// New returns the marshaled bytes of the default value for the component struct.
	New() ([]byte, error)
	// Name returns the name of the component.
	Name() string

	Decode([]byte) (any, error)
	Encode(any) ([]byte, error)
}

type ComponentTypeID int

type (
	// Index represents the Index of component in an archetype.
	ComponentIndex int
)
