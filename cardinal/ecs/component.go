package ecs

import (
	"fmt"
	"reflect"
	"unsafe"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
)

// IComponentType is an interface for component types.
type IComponentType = component.IComponentType

// NewComponentType creates a new component type.
// The function is used to create a new component of the type.
func NewComponentType[T any](opts ...ComponentOption[T]) *ComponentType[T] {
	var t T
	comp := newComponentType(t, nil)
	for _, opt := range opts {
		opt(comp)
	}
	return comp
}

// ComponentType represents a type of component. It is used to identify
// a component when getting or setting componentStore of an entity.
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
		return fmt.Errorf("id for component %v is already set to %v, cannot change to %v", c, c.id, id)
	}
	c.id = id
	c.isIDSet = true
	return nil
}

// Get returns component data from the entity.
func (c *ComponentType[T]) Get(w *World, id storage.EntityID) (comp T, err error) {
	entity, err := w.Entity(id)
	if err != nil {
		return comp, err
	}
	bz, err := entity.Component(w, c)
	if err != nil {
		return comp, err
	}
	return storage.Decode[T](bz)
}

// Update is a helper that combines a Get followed by a Set to modify a component's value. Pass in a function
// fn that will return a modified component. Update will hide the calls to Get and Set
func (c *ComponentType[T]) Update(w *World, id storage.EntityID, fn func(T) T) error {
	val, err := c.Get(w, id)
	if err != nil {
		return err
	}
	val = fn(val)
	return c.Set(w, id, val)
}

// Set sets component data to the entity.
func (c *ComponentType[T]) Set(w *World, id storage.EntityID, component T) error {
	entity, err := w.Entity(id)
	if err != nil {
		return err
	}
	bz, err := storage.Encode(component)
	if err != nil {
		return err
	}
	err = w.SetComponent(c, bz, entity.Loc.ArchID, entity.Loc.CompIndex)
	if err != nil {
		return err
	}
	return nil
}

// Each iterates over the entityLocationStore that have the component.
func (c *ComponentType[T]) Each(w *World, callback func(storage.EntityID)) {
	c.query.Each(w, callback)
}

// First returns the first entity that has the component.
func (c *ComponentType[T]) First(w *World) (storage.EntityID, bool, error) {
	return c.query.First(w)
}

// MustFirst returns the first entity that has the component or panics.
func (c *ComponentType[T]) MustFirst(w *World) (storage.EntityID, error) {
	id, ok, err := c.query.First(w)
	if err != nil {
		return storage.BadID, err
	}
	if !ok {
		panic(fmt.Sprintf("no entity has the component %s", c.name))
	}

	return id, nil
}

// RemoveFrom removes this component form the given entity.
func (c *ComponentType[T]) RemoveFrom(w *World, id storage.EntityID) error {
	e, err := w.Entity(id)
	if err != nil {
		return err
	}
	return e.RemoveComponent(w, c)
}

// AddTo adds this component to the given entity.
func (c *ComponentType[T]) AddTo(w *World, id storage.EntityID) error {
	e, err := w.Entity(id)
	if err != nil {
		return err
	}
	return e.AddComponent(w, c)
}

// SetValue sets the value of the component.
func (c *ComponentType[T]) SetValue(w *World, id storage.EntityID, value T) error {
	_, err := c.Get(w, id)
	if err != nil {
		return err
	}
	return c.Set(w, id, value)
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
	return storage.Encode(comp)
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
func newComponentType[T any](s T, defaultVal interface{}) *ComponentType[T] {
	componentType := &ComponentType[T]{
		typ:        reflect.TypeOf(s),
		name:       reflect.TypeOf(s).Name(),
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
