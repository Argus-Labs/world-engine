package storage

import (
	"fmt"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/ecs/codec"
	"reflect"
)

var (
	nextMockComponentTypeID component.TypeID = 1
)

type MockComponentType[T any] struct {
	id         component.TypeID
	typ        reflect.Type
	defaultVal interface{}
}

func NewMockComponentType[T any](t T, defaultVal interface{}) *MockComponentType[T] {
	m := &MockComponentType[T]{
		id:         nextMockComponentTypeID,
		typ:        reflect.TypeOf(t),
		defaultVal: defaultVal,
	}
	nextMockComponentTypeID++
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
		comp, _ = m.defaultVal.(T)
	}
	return codec.Encode(comp)
}

func (m *MockComponentType[T]) Name() string {
	return fmt.Sprintf("%s[%s]", reflect.TypeOf(m).Name(), m.typ.Name())
}

var _ component.ComponentMetadata = &MockComponentType[int]{}

func (m *MockComponentType[T]) Decode(bytes []byte) (any, error) {
	return codec.Decode[T](bytes)
}

func (m *MockComponentType[T]) Encode(a any) ([]byte, error) {
	return codec.Encode(a)
}
