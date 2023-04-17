package storage

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"reflect"
	"unsafe"

	"github.com/argus-labs/cardinal/ECS/component"
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

func (m *MockComponentType[T]) ID() component.TypeID {
	return m.id
}

func (m *MockComponentType[T]) New() ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	var comp T
	if m.defaultVal != nil {
		comp = m.defaultVal.(T)
	}
	err := enc.Encode(comp)
	return buf.Bytes(), err
}

func (m *MockComponentType[T]) setDefaultVal(ptr unsafe.Pointer) {
	v := reflect.Indirect(reflect.ValueOf(m.defaultVal))
	reflect.NewAt(m.typ, ptr).Elem().Set(v)
}

func (m *MockComponentType[T]) Name() string {
	return fmt.Sprintf("%s[%s]", reflect.TypeOf(m).Name(), m.typ.Name())
}
