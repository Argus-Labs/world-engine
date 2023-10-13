package storage

import (
	"fmt"
	"reflect"
	"unsafe"

	"pkg.world.dev/world-engine/cardinal/ecs/codec"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

var (
	nextMockComponentTypeId component.TypeID = 1
)

type MockComponentType[T any] struct {
	id         component.TypeID
	typ        reflect.Type
	defaultVal interface{}
}

func NewMockComponentType[T any](t T, defaultVal interface{}) *MockComponentType[T] {
	m := &MockComponentType[T]{
		id:         nextMockComponentTypeId,
		typ:        reflect.TypeOf(t),
		defaultVal: defaultVal,
	}
	nextMockComponentTypeId++
	return m
}

func (m *MockComponentType[T]) SetID(id component.TypeID) error {
	m.id = id
	return nil
}

func (m *MockComponentType[T]) ID() component.TypeID {
	return m.id
}

func (m *MockComponentType[T]) New() ([]byte, error) {
	var comp T
	if m.defaultVal != nil {
		comp = m.defaultVal.(T)
	}
	return codec.Encode(comp)
}

func (m *MockComponentType[T]) setDefaultVal(ptr unsafe.Pointer) {
	v := reflect.Indirect(reflect.ValueOf(m.defaultVal))
	reflect.NewAt(m.typ, ptr).Elem().Set(v)
}

func (m *MockComponentType[T]) Name() string {
	return fmt.Sprintf("%s[%s]", reflect.TypeOf(m).Name(), m.typ.Name())
}

var _ component.IComponentMetaData = &MockComponentType[int]{}

func (m *MockComponentType[T]) Decode(bytes []byte) (any, error) {
	return codec.Decode[T](bytes)
}

func (m *MockComponentType[T]) Encode(a any) ([]byte, error) {
	return codec.Encode(a)
}
