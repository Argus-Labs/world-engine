package ECS

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"reflect"
	"unsafe"

	"github.com/argus-labs/cardinal/ECS/component"
	"github.com/argus-labs/cardinal/ECS/filter"
	"github.com/argus-labs/cardinal/ECS/storage"
)

// IComponentType is an interface for component types.
type IComponentType = component.IComponentType

// NewComponentType creates a new component type.
// The function is used to create a new component of the type.
// It receives a function that returns a pointer to a new component.
// The first argument is a default value of the component.
func NewComponentType[T any](opts ...interface{}) *ComponentType[T] {
	var t T
	if len(opts) == 0 {
		return newComponentType(t, nil)
	}
	return newComponentType(t, opts[0])
}

// ComponentType represents a type of component. It is used to identify
// a component when getting or setting componentStore of an entity.
type ComponentType[T any] struct {
	w          storage.WorldAccessor
	id         component.TypeID
	typ        reflect.Type
	name       string
	defaultVal interface{}
	query      *Query
}

func (c *ComponentType[T]) Initialize(w storage.WorldAccessor) {
	c.w = w
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

// Get returns component data from the entry.
func (c *ComponentType[T]) Get(entry *storage.Entry) (T, error) {
	bz, _ := entry.Component(c.w, c)
	comp, err := decodeComponent[T](bz)
	return comp, err
}

// Set sets component data to the entry.
func (c *ComponentType[T]) Set(entry *storage.Entry, component *T) error {
	bz, err := encodeComponent[T](*component)
	if err != nil {
		return err
	}
	c.w.SetComponent(c, bz, entry.Loc.ArchIndex, entry.Loc.CompIndex)
	return nil
}

// Each iterates over the entityLocationStore that have the component.
func (c *ComponentType[T]) Each(w World, callback func(*storage.Entry)) {
	c.query.Each(w, callback)
}

// First returns the first entity that has the component.
func (c *ComponentType[T]) First(w World) (*storage.Entry, bool, error) {
	return c.query.First(w)
}

// MustFirst returns the first entity that has the component or panics.
func (c *ComponentType[T]) MustFirst(w World) *storage.Entry {
	e, ok, _ := c.query.First(w)
	if !ok {
		panic(fmt.Sprintf("no entity has the component %s", c.name))
	}

	return e
}

// SetValue sets the value of the component.
func (c *ComponentType[T]) SetValue(entry *storage.Entry, value T) error {
	_, err := c.Get(entry)
	if err != nil {
		return err
	}
	return c.Set(entry, &value)
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

// NewComponentType creates a new component type.
// The argument is a struct that represents a data of the component.
func newComponentType[T any](s T, defaultVal interface{}) *ComponentType[T] {
	componentType := &ComponentType[T]{
		id:         nextComponentTypeId,
		typ:        reflect.TypeOf(s),
		name:       reflect.TypeOf(s).Name(),
		defaultVal: defaultVal,
	}
	componentType.query = NewQuery(filter.Contains(componentType))
	if defaultVal != nil {
		componentType.validateDefaultVal()
	}
	nextComponentTypeId++
	return componentType
}
