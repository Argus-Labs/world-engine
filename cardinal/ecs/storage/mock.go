package storage

import (
	"errors"
	"fmt"
	"reflect"
	"unsafe"

	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

var (
	nextMockComponentTypeId component.TypeID = 1
)

type MockComponentType[T any] struct {
	id         component.TypeID
	typ        reflect.Type
	defaultVal interface{}
	isIdSet    bool
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

func (m *MockComponentType[T]) IsIDSet() bool {
	return m.isIdSet
}

func (m *MockComponentType[T]) SetID(id component.TypeID) error {
	m.id = id
	if m.isIdSet {
		return errors.New("id already set on component")
	}
	m.isIdSet = true
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
	return Encode(comp)
}

func (m *MockComponentType[T]) setDefaultVal(ptr unsafe.Pointer) {
	v := reflect.Indirect(reflect.ValueOf(m.defaultVal))
	reflect.NewAt(m.typ, ptr).Elem().Set(v)
}

func (m *MockComponentType[T]) Name() string {
	return fmt.Sprintf("%s[%s]", reflect.TypeOf(m).Name(), m.typ.Name())
}
