package gamestate

import (
	"errors"

	"github.com/rotisserie/eris"
)

var _ VolatileStorage[string, any] = &MapStorage[string, any]{}

var ErrNotFound = errors.New("key not found in map")

type MapStorage[K comparable, V any] struct {
	internalMap map[K]V
}

func NewMapStorage[K comparable, V any]() *MapStorage[K, V] {
	return &MapStorage[K, V]{
		internalMap: make(map[K]V),
	}
}

func (m *MapStorage[K, V]) Keys() ([]K, error) {
	acc := make([]K, 0, len(m.internalMap))
	for k := range m.internalMap {
		acc = append(acc, k)
	}
	return acc, nil
}

func (m *MapStorage[K, V]) Delete(key K) error {
	delete(m.internalMap, key)
	return nil
}

func (m *MapStorage[K, V]) Get(key K) (V, error) {
	v, ok := m.internalMap[key]
	if !ok {
		return v, eris.Wrap(ErrNotFound, "")
	}
	return v, nil
}

func (m *MapStorage[K, V]) Set(key K, value V) error {
	m.internalMap[key] = value
	return nil
}

func (m *MapStorage[K, V]) Clear() error {
	m.internalMap = make(map[K]V)
	return nil
}

func (m *MapStorage[K, V]) Len() int {
	return len(m.internalMap)
}
