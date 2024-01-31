package component

import (
	"fmt"
	"pkg.world.dev/world-engine/cardinal/codec"
	"reflect"

	"github.com/invopop/jsonschema"
	"github.com/rotisserie/eris"
	"github.com/wI2L/jsondiff"
)

type TypeID int

// Component is the interface that the user needs to implement to create a new component type.
type Component interface {
	// Name returns the name of the component.
	Name() string
}

// ComponentMetadata wraps the user-defined Component struct and provides functionalities that is used internally
// in the engine.
type ComponentMetadata interface { //revive:disable-line:exported
	// SetID sets the ID of this component. It must only be set once
	SetID(TypeID) error
	// ID returns the ID of the component.
	ID() TypeID
	// New returns the marshaled bytes of the default value for the component struct.
	New() ([]byte, error)
	Encode(any) ([]byte, error)
	Decode([]byte) (any, error)
	GetSchema() []byte

	Component
}

// NewComponentMetadata creates a new component type.
// The function is used to create a new component of the type.
func NewComponentMetadata[T Component](opts ...ComponentOption[T]) (ComponentMetadata, error) {
	var t T
	comp, err := newComponentType(t, t.Name(), nil)
	if err != nil {
		return comp, err
	}
	for _, opt := range opts {
		opt(comp)
	}
	return comp, nil
}

// componentMetadata represents a type of component. It is used to identify
// a component when getting or setting the component of an entity.
type componentMetadata[T any] struct {
	isIDSet    bool
	id         TypeID
	typ        reflect.Type
	name       string
	defaultVal interface{}
	schema     []byte
}

func (c *componentMetadata[T]) GetSchema() []byte {
	return c.schema
}

// SetID set's this component's ID. It must be unique across the world object.
func (c *componentMetadata[T]) SetID(id TypeID) error {
	if c.isIDSet {
		// In games implemented with Cardinal, components will only be initialized one time (on startup).
		// In tests, it's often useful to use the same component in multiple worlds. This check will allow for the
		// re-initialization of components, as long as the ID doesn't change.
		if id == c.id {
			return nil
		}
		return eris.Errorf("id for component %v is already set to %v, cannot change to %v", c, c.id, id)
	}
	c.id = id
	c.isIDSet = true
	return nil
}

// String returns the component type name.
func (c *componentMetadata[T]) String() string {
	return c.name
}

// Name returns the component type name.
func (c *componentMetadata[T]) Name() string {
	return c.name
}

// ID returns the component type id.
func (c *componentMetadata[T]) ID() TypeID {
	return c.id
}

func (c *componentMetadata[T]) New() ([]byte, error) {
	var comp T
	var ok bool
	if c.defaultVal != nil {
		comp, ok = c.defaultVal.(T)
		if !ok {
			return nil, eris.Errorf("could not convert %T to %T", c.defaultVal, new(T))
		}
	}
	return codec.Encode(comp)
}

func (c *componentMetadata[T]) Encode(v any) ([]byte, error) {
	return codec.Encode(v)
}

func (c *componentMetadata[T]) Decode(bz []byte) (any, error) {
	return codec.Decode[T](bz)
}

func (c *componentMetadata[T]) validateDefaultVal() {
	if !reflect.TypeOf(c.defaultVal).AssignableTo(c.typ) {
		errString := fmt.Sprintf("default value is not assignable to component type: %s", c.name)
		panic(errString)
	}
}

// newComponentType creates a new component type.
// The argument is a struct that represents a data of the component.
func newComponentType[T Component](s T, name string, defaultVal interface{}) (*componentMetadata[T], error) {
	schema, err := SerializeComponentSchema(s)
	if err != nil {
		return nil, err
	}
	componentType := &componentMetadata[T]{
		typ:        reflect.TypeOf(s),
		name:       name,
		defaultVal: defaultVal,
		schema:     schema,
	}
	if defaultVal != nil {
		componentType.validateDefaultVal()
	}
	return componentType, nil
}

// ComponentOption is a type that can be passed to NewComponentMetadata to augment the creation
// of the component type.
type ComponentOption[T any] func(c *componentMetadata[T]) //revive:disable-line:exported

// WithDefault updated the created componentMetadata with a default value.
func WithDefault[T any](defaultVal T) ComponentOption[T] {
	return func(c *componentMetadata[T]) {
		c.defaultVal = defaultVal
		c.validateDefaultVal()
	}
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
