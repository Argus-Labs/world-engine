package storage

import (
	"fmt"
	"github.com/goccy/go-json"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/types/component"
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
	return m.Encode(comp)
}

func (m *MockComponentType[T]) Name() string {
	return fmt.Sprintf("%s[%s]", reflect.TypeOf(m).Name(), m.typ.Name())
}

var _ component.ComponentMetadata = &MockComponentType[int]{}

func (m *MockComponentType[T]) Decode(bz []byte) (any, error) {
	comp := new(T)
	err := json.Unmarshal(bz, comp)
	if err != nil {
		return *comp, eris.Wrap(err, "")
	}
	return *comp, nil
}

func (m *MockComponentType[T]) Encode(v any) ([]byte, error) {
	bz, err := json.Marshal(v)
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	return bz, nil
}
