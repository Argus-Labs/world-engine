package component_metadata

import (
	"fmt"
	"reflect"
	"unsafe"

	"pkg.world.dev/world-engine/cardinal/ecs/codec"
)

type (
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

// NewComponentMetadata creates a new component type.
// The function is used to create a new component of the type.
func NewComponentMetadata[T Component](opts ...ComponentOption[T]) *ComponentMetaData[T] {
	var t T
	comp := newComponentType(t, t.Name(), nil)
	for _, opt := range opts {
		opt(comp)
	}
	return comp
}

// ComponentMetaData represents a type of component. It is used to identify
// a component when getting or setting the component of an entity.
type ComponentMetaData[T any] struct {
	isIDSet    bool
	id         TypeID
	typ        reflect.Type
	name       string
	defaultVal interface{}
}

// SetID set's this component's ID. It must be unique across the world object.
func (c *ComponentMetaData[T]) SetID(id TypeID) error {
	if c.isIDSet {
		// In games implemented with Cardinal, components will only be initialized one time (on startup).
		// In tests, it's often useful to use the same component in multiple worlds. This check will allow for the
		// re-initialization of components, as long as the ID doesn't change.
		if id == c.id {
			return nil
		}
		return fmt.Errorf("id for component %v is already set to %v, cannot change to %v", c, c.id, id)
	}
	c.id = id
	c.isIDSet = true
	return nil
}

// String returns the component type name.
func (c *ComponentMetaData[T]) String() string {
	return c.name
}

// SetName sets the component type name.
func (c *ComponentMetaData[T]) SetName(name string) *ComponentMetaData[T] {
	c.name = name
	return c
}

// Name returns the component type name.
func (c *ComponentMetaData[T]) Name() string {
	return c.name
}

// ID returns the component type id.
func (c *ComponentMetaData[T]) ID() TypeID {
	return c.id
}

func (c *ComponentMetaData[T]) New() ([]byte, error) {
	var comp T
	if c.defaultVal != nil {
		comp = c.defaultVal.(T)
	}
	return codec.Encode(comp)
}

func (c *ComponentMetaData[T]) Encode(v any) ([]byte, error) {
	return codec.Encode(v)
}

func (c *ComponentMetaData[T]) Decode(bz []byte) (any, error) {
	return codec.Decode[T](bz)
}

func (c *ComponentMetaData[T]) setDefaultVal(ptr unsafe.Pointer) {
	v := reflect.Indirect(reflect.ValueOf(c.defaultVal))
	reflect.NewAt(c.typ, ptr).Elem().Set(v)
}

func (c *ComponentMetaData[T]) validateDefaultVal() {
	if !reflect.TypeOf(c.defaultVal).AssignableTo(c.typ) {
		err := fmt.Sprintf("default value is not assignable to component type: %s", c.name)
		panic(err)
	}
}

// newComponentType creates a new component type.
// The argument is a struct that represents a data of the component.
func newComponentType[T any](s T, name string, defaultVal interface{}) *ComponentMetaData[T] {
	componentType := &ComponentMetaData[T]{
		typ:        reflect.TypeOf(s),
		name:       name,
		defaultVal: defaultVal,
	}
	if defaultVal != nil {
		componentType.validateDefaultVal()
	}
	return componentType
}

// ComponentOption is a type that can be passed to NewComponentMetadata to augment the creation
// of the component type
type ComponentOption[T any] func(c *ComponentMetaData[T])

// WithDefault updated the created ComponentMetaData with a default value
func WithDefault[T any](defaultVal T) ComponentOption[T] {
	return func(c *ComponentMetaData[T]) {
		c.defaultVal = defaultVal
		c.validateDefaultVal()
	}
}
