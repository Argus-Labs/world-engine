package gamestate

import (
	"fmt"
	"pkg.world.dev/world-engine/cardinal/codec"
	"pkg.world.dev/world-engine/cardinal/types"
	"reflect"
)

var (
	nextMockComponentTypeID types.ComponentID = 1
)

type MockComponentType[T any] struct {
	id         types.ComponentID
	typ        reflect.Type
	defaultVal interface{}
	schema     []byte
}

func NewMockComponentType[T types.Component](t T, defaultVal interface{}) (*MockComponentType[T], error) {
	schema, err := types.SerializeComponentSchema(t)
	if err != nil {
		return nil, err
	}
	m := &MockComponentType[T]{
		id:         nextMockComponentTypeID,
		typ:        reflect.TypeOf(t),
		defaultVal: defaultVal,
		schema:     schema,
	}
	nextMockComponentTypeID++
	return m, nil
}

func (m *MockComponentType[T]) SetID(id types.ComponentID) error {
	m.id = id
	return nil
}

func (m *MockComponentType[T]) ID() types.ComponentID {
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

var _ types.ComponentMetadata = &MockComponentType[int]{}

func (m *MockComponentType[T]) Decode(bytes []byte) (any, error) {
	return codec.Decode[T](bytes)
}

func (m *MockComponentType[T]) Encode(a any) ([]byte, error) {
	return codec.Encode(a)
}

func (m *MockComponentType[T]) GetSchema() []byte {
	return m.schema
}
