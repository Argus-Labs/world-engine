package ecs

import (
	"fmt"
	"reflect"
	"strconv"
	"unsafe"

	"pkg.world.dev/world-engine/cardinal/ecs/codec"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
)

// IComponentType is an interface for component types.
type IComponentType = component.IComponentType

// NewComponentType creates a new component type.
// The function is used to create a new component of the type.
func NewComponentType[T any](name string, opts ...ComponentOption[T]) *ComponentType[T] {
	var t T
	comp := newComponentType(t, name, nil)
	for _, opt := range opts {
		opt(comp)
	}
	return comp
}

// ComponentType represents a type of component. It is used to identify
// a component when getting or setting the component of an entity.
type ComponentType[T any] struct {
	isIDSet    bool
	id         component.TypeID
	typ        reflect.Type
	name       string
	defaultVal interface{}
	query      *Query
}

// SetID set's this component's ID. It must be unique across the world object.
func (c *ComponentType[T]) SetID(id component.TypeID) error {
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

// Get returns component data from the entity.
func (c *ComponentType[T]) Get(w *World, id entity.ID) (comp T, err error) {
	value, err := w.StoreManager().GetComponentForEntity(c, id)
	if err != nil {
		return comp, err
	}
	comp, ok := value.(T)
	if !ok {
		return comp, fmt.Errorf("type assertion for component failed: %v to %v", value, c)
	}
	return comp, nil
}

// Update is a helper that combines a Get followed by a Set to modify a component's value. Pass in a function
// fn that will return a modified component. Update will hide the calls to Get and Set
func (c *ComponentType[T]) Update(w *World, id entity.ID, fn func(T) T) error {
	val, err := c.Get(w, id)
	if err != nil {
		return err
	}
	val = fn(val)
	return c.Set(w, id, val)
}

// Set sets component data to the entity.
func (c *ComponentType[T]) Set(w *World, id entity.ID, component T) error {
	err := w.StoreManager().SetComponentForEntity(c, id, component)
	if err != nil {
		return err
	}
	w.Logger.Debug().
		Str("entity_id", strconv.FormatUint(uint64(id), 10)).
		Str("component_name", c.name).
		Int("component_id", int(c.ID())).
		Msg("entity updated")
	return nil
}

// Each iterates over the entities that have the component.
// If you would like to stop the iteration, return false to the callback. To continue iterating, return true.
func (c *ComponentType[T]) Each(w *World, callback QueryCallBackFn) {
	c.query.Each(w, callback)
}

// First returns the first entity that has the component.
func (c *ComponentType[T]) First(w *World) (entity.ID, error) {
	return c.query.First(w)
}

// MustFirst returns the first entity that has the component or panics.
func (c *ComponentType[T]) MustFirst(w *World) entity.ID {
	id, err := c.query.First(w)
	if err != nil {
		panic(fmt.Sprintf("no entity has the component %s", c.name))
	}
	return id
}

// RemoveFrom removes this component from the given entity.
func (c *ComponentType[T]) RemoveFrom(w *World, id entity.ID) error {
	return w.StoreManager().RemoveComponentFromEntity(c, id)
}

// AddTo adds this component to the given entity.
func (c *ComponentType[T]) AddTo(w *World, id entity.ID) error {
	return w.StoreManager().AddComponentToEntity(c, id)
}

// String returns the component type name.
func (c *ComponentType[T]) String() string {
	return c.name
}

// SetName sets the component type name.
func (c *ComponentType[T]) SetName(name string) *ComponentType[T] {
	c.name = name
	return c
}

// Name returns the component type name.
func (c *ComponentType[T]) Name() string {
	return c.name
}

// ID returns the component type id.
func (c *ComponentType[T]) ID() component.TypeID {
	return c.id
}

func (c *ComponentType[T]) New() ([]byte, error) {
	var comp T
	if c.defaultVal != nil {
		comp = c.defaultVal.(T)
	}
	return codec.Encode(comp)
}

func (c *ComponentType[T]) Encode(v any) ([]byte, error) {
	return codec.Encode(v)
}

func (c *ComponentType[T]) Decode(bz []byte) (any, error) {
	return codec.Decode[T](bz)
}

func (c *ComponentType[T]) setDefaultVal(ptr unsafe.Pointer) {
	v := reflect.Indirect(reflect.ValueOf(c.defaultVal))
	reflect.NewAt(c.typ, ptr).Elem().Set(v)
}

func (c *ComponentType[T]) validateDefaultVal() {
	if !reflect.TypeOf(c.defaultVal).AssignableTo(c.typ) {
		err := fmt.Sprintf("default value is not assignable to component type: %s", c.name)
		panic(err)
	}
}

// newComponentType creates a new component type.
// The argument is a struct that represents a data of the component.
func newComponentType[T any](s T, name string, defaultVal interface{}) *ComponentType[T] {
	componentType := &ComponentType[T]{
		typ:        reflect.TypeOf(s),
		name:       name,
		defaultVal: defaultVal,
	}
	componentType.query = NewQuery(filter.Contains(componentType))
	if defaultVal != nil {
		componentType.validateDefaultVal()
	}
	return componentType
}

// ComponentOption is a type that can be passed to NewComponentType to augment the creation
// of the component type
type ComponentOption[T any] func(c *ComponentType[T])

// WithDefault updated the created ComponentType with a default value
func WithDefault[T any](defaultVal T) ComponentOption[T] {
	return func(c *ComponentType[T]) {
		c.defaultVal = defaultVal
		c.validateDefaultVal()
	}
}
