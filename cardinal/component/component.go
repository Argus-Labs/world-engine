package component

import (
	"fmt"
	"reflect"

	"github.com/invopop/jsonschema"
	"github.com/rotisserie/eris"
	"github.com/wI2L/jsondiff"

	"pkg.world.dev/world-engine/cardinal/codec"
	"pkg.world.dev/world-engine/cardinal/types"
)

// Interface guard
var _ types.ComponentMetadata = (*componentMetadata[types.Component])(nil)

// Option is a type that can be passed to NewComponentMetadata to augment the creation
// of the component type.
type Option[T types.Component] func(c *componentMetadata[T])

// componentMetadata represents a type of component. It is used to identify
// a component when getting or setting the component of an entity.
type componentMetadata[T types.Component] struct {
	isIDSet    bool
	id         types.ComponentID
	compType   reflect.Type
	name       string
	schema     []byte
	defaultVal types.Component
}

// NewComponentMetadata creates a new component type.
// The function is used to create a new component of the type.
func NewComponentMetadata[T types.Component](opts ...Option[T]) (
	types.ComponentMetadata, error,
) {
	var t T
	compType := reflect.TypeOf(t)

	schema, err := jsonschema.ReflectFromType(compType).MarshalJSON()
	if err != nil {
		return nil, eris.Wrap(err, "component must be json serializable")
	}

	compMetadata := &componentMetadata[T]{
		compType: compType,
		name:     t.Name(),
		schema:   schema,
	}
	for _, opt := range opts {
		opt(compMetadata)
	}

	return compMetadata, nil
}

func (c *componentMetadata[T]) GetSchema() []byte {
	return c.schema
}

// SetID set's this component's ID. It must be unique across the world object.
func (c *componentMetadata[T]) SetID(id types.ComponentID) error {
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
func (c *componentMetadata[T]) ID() types.ComponentID {
	return c.id
}

func (c *componentMetadata[T]) New() ([]byte, error) {
	if c.defaultVal != nil {
		return codec.Encode(c.defaultVal)
	}
	return codec.Encode(c.compType)
}

func (c *componentMetadata[T]) Encode(v any) ([]byte, error) {
	return codec.Encode(v)
}

func (c *componentMetadata[T]) Decode(bz []byte) (types.Component, error) {
	return codec.Decode[T](bz)
}

func (c *componentMetadata[T]) ValidateAgainstSchema(targetSchema []byte) error {
	diff, err := jsondiff.CompareJSON(c.schema, targetSchema)
	if err != nil {
		return eris.Wrap(err, "failed to compare component schema")
	}

	if diff.String() != "" {
		return eris.Wrap(types.ErrComponentSchemaMismatch, diff.String())
	}

	return nil
}

func (c *componentMetadata[T]) validateDefaultVal() {
	if !reflect.TypeOf(c.defaultVal).AssignableTo(c.compType) {
		panic(fmt.Sprintf("default value is not assignable to component type: %s", c.name))
	}
}

// WithDefault updated the created componentMetadata with a default value.
func WithDefault[T types.Component](defaultVal T) Option[T] {
	return func(c *componentMetadata[T]) {
		c.defaultVal = defaultVal
		c.validateDefaultVal()
	}
}
