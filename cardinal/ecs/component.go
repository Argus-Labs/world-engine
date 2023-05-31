package ecs

import (
	"bytes"
	"encoding/gob"
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
func NewComponentType[T any](world *World, opts ...ComponentOption[T]) *ComponentType[T] {
	var t T
	comp := newComponentType(t, world, nil)
	for _, opt := range opts {
		opt(comp)
	}
	return comp
}

// ComponentType represents a type of component. It is used to identify
// a component when getting or setting componentStore of an entity.
type ComponentType[T any] struct {
	w          *World
	id         component.TypeID
	typ        reflect.Type
	name       string
	defaultVal interface{}
	query      *Query
}

func decodeComponent[T any](bz []byte) (T, error) {
	var buf bytes.Buffer
	buf.Write(bz)
	dec := gob.NewDecoder(&buf)
	comp := new(T)
	err := dec.Decode(comp)
	return *comp, err
}

func encodeComponent[T any](comp T) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(comp)
	return buf.Bytes(), err
}

// Get returns component data from the entity.
func (c *ComponentType[T]) Get(id storage.EntityID) (comp T, err error) {
	entity, err := c.w.Entity(id)
	if err != nil {
		return comp, err
	}
	bz, err := entity.Component(c.w, c)
	if err != nil {
		return comp, err
	}
	comp, err = decodeComponent[T](bz)
	return comp, err
}

// Update is a helper that combines a Get followed by a Set to modify a component's value. Pass in a function
// fn that will modify a component. Update will hide the calls to Get and Set
func (c *ComponentType[T]) Update(id storage.EntityID, fn func(*T)) error {
	val, err := c.Get(id)
	if err != nil {
		return err
	}
	fn(&val)
	return c.Set(id, &val)
}

// Set sets component data to the entity.
func (c *ComponentType[T]) Set(id storage.EntityID, component *T) error {
	entity, err := c.w.Entity(id)
	if err != nil {
		return err
	}
	bz, err := encodeComponent[T](*component)
	if err != nil {
		return err
	}
	err = c.w.SetComponent(c, bz, entity.Loc.ArchIndex, entity.Loc.CompIndex)
	if err != nil {
		return err
	}
	return nil
}

// Each iterates over the entityLocationStore that have the component.
func (c *ComponentType[T]) Each(callback func(storage.EntityID)) {
	c.query.Each(c.w, callback)
}

// First returns the first entity that has the component.
func (c *ComponentType[T]) First() (storage.EntityID, bool, error) {
	return c.query.First(c.w)
}

// MustFirst returns the first entity that has the component or panics.
func (c *ComponentType[T]) MustFirst() (storage.EntityID, error) {
	id, ok, err := c.query.First(c.w)
	if err != nil {
		return storage.BadID, err
	}
	if !ok {
		panic(fmt.Sprintf("no entity has the component %s", c.name))
	}

	return id, nil
}

// RemoveFrom removes this component form the given entity.
func (c *ComponentType[T]) RemoveFrom(id storage.EntityID) error {
	e, err := c.w.Entity(id)
	if err != nil {
		return err
	}
	return e.RemoveComponent(c.w, c)
}

// AddTo adds this component to the given entity.
func (c *ComponentType[T]) AddTo(id storage.EntityID) error {
	e, err := c.w.Entity(id)
	if err != nil {
		return err
	}
	return e.AddComponent(c.w, c)
}

// SetValue sets the value of the component.
func (c *ComponentType[T]) SetValue(id storage.EntityID, value T) error {
	_, err := c.Get(id)
	if err != nil {
		return err
	}
	return c.Set(id, &value)
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
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	var comp T
	if c.defaultVal != nil {
		comp = c.defaultVal.(T)
	}
	err := enc.Encode(comp)
	return buf.Bytes(), err
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

// TODO: this should be handled by storage.
var nextComponentTypeId component.TypeID = 1

// newComponentType creates a new component type.
// The argument is a struct that represents a data of the component.
func newComponentType[T any](s T, world *World, defaultVal interface{}) *ComponentType[T] {
	componentType := &ComponentType[T]{
		id:         nextComponentTypeId,
		typ:        reflect.TypeOf(s),
		name:       reflect.TypeOf(s).Name(),
		defaultVal: defaultVal,
		w:          world,
	}
	componentType.query = NewQuery(filter.Contains(componentType))
	if defaultVal != nil {
		componentType.validateDefaultVal()
	}
	nextComponentTypeId++
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
