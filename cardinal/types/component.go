package types

import (
	"github.com/invopop/jsonschema"
	"github.com/rotisserie/eris"
	"github.com/wI2L/jsondiff"
)

type ComponentID int

// Component is the interface that the user needs to implement to create a new component type.
type Component interface {
	// Name returns the name of the component.
	Name() string
}

// ComponentMetadata wraps the user-defined Component struct and provides functionalities that is used internally
// in the engine.
type ComponentMetadata interface { //revive:disable-line:exported
	// SetID sets the ArchetypeID of this component. It must only be set once
	SetID(ComponentID) error
	// ArchetypeID returns the ArchetypeID of the component.
	ID() ComponentID
	// New returns the marshaled bytes of the default value for the component struct.
	New() ([]byte, error)
	Encode(any) ([]byte, error)
	Decode([]byte) (any, error)
	GetSchema() []byte

	Component
}

func SerializeComponentSchema(component Component) ([]byte, error) {
	componentSchema := jsonschema.Reflect(component)
	schema, err := componentSchema.MarshalJSON()
	if err != nil {
		return nil, eris.Wrap(err, "component must be json serializable")
	}
	return schema, nil
}

func IsComponentValid(component Component, jsonSchemaBytes []byte) (bool, error) {
	componentSchema := jsonschema.Reflect(component)
	componentSchemaBytes, err := componentSchema.MarshalJSON()
	if err != nil {
		return false, eris.Wrap(err, "")
	}
	return IsSchemaValid(componentSchemaBytes, jsonSchemaBytes)
}

func IsSchemaValid(jsonSchemaBytes1 []byte, jsonSchemaBytes2 []byte) (bool, error) {
	patch, err := jsondiff.CompareJSON(jsonSchemaBytes1, jsonSchemaBytes2)
	if err != nil {
		return false, eris.Wrap(err, "")
	}
	return patch.String() == "", nil
}

// ConvertComponentMetadatasToComponents Cast an array of ComponentMetadata into an array of Component
func ConvertComponentMetadatasToComponents(comps []ComponentMetadata) []Component {
	ret := make([]Component, len(comps))
	for i, comp := range comps {
		ret[i] = comp
	}
	return ret
}
