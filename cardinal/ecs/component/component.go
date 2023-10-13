package component

type (
	// Index represents the Index of component in an archetype.
	Index int

	TypeID int

	// IComponentMetaData is a high level representation of a user defined component struct.
	IComponentMetaData interface {
		// SetID sets the ID of this component. It must only be set once
		SetID(TypeID) error
		// ID returns the ID of the component.
		ID() TypeID
		// New returns the marshaled bytes of the default value for the component struct.
		New() ([]byte, error)

		Encode(any) ([]byte, error)
		Decode([]byte) (any, error)
		Name() string
	}

	Component interface {
		// Name returns the name of the component.
		Name() string
	}
)

// Contains returns true if the given slice of components contains the given component. Components are the same if they
// have the same ID.
func Contains(components []IComponentMetaData, cType IComponentMetaData) bool {
	for _, c := range components {
		if cType.ID() == c.ID() {
			return true
		}
	}
	return false
}
