package gamestate

import (
	"context"

	"github.com/rotisserie/eris"
)

type MapStorage[K comparable, V any] struct {
	internalMap map[K]V
}

func NewMapStorage[K comparable, V any]() *MapStorage[K, V] {
	return &MapStorage[K, V]{
		internalMap: make(map[K]V),
	}
}

func (m *MapStorage[K, V]) Keys(_ context.Context) ([]K, error) {
	res := make([]K, 0)
	for key := range m.internalMap {
		res = append(res, key)
	}
	return res, nil
}

func (m *MapStorage[K, V]) Get(_ context.Context, key K) (any, error) {
	var resAny any
	res, ok := m.internalMap[key]
	if !ok {
		return nil, eris.New("key does not exist in storage")
	}
	resAny = res
	return resAny, nil
}

func (m *MapStorage[K, V]) Clear(_ context.Context) error {
	m.internalMap = make(map[K]V)
	return nil
}

func (m *MapStorage[K, V]) GetFloat64(_ context.Context, key K) (float64, error) {
	var res float64
	var resAny any
	resRaw, ok := m.internalMap[key]
	if !ok {
		return 0, eris.New("does not exist in map")
	}
	resAny = resRaw
	res, ok = resAny.(float64)
	if !ok {
		return 0, eris.New("cannot convert to float64")
	}
	return res, nil
}
func (m *MapStorage[K, V]) GetFloat32(_ context.Context, key K) (float32, error) {
	var res float32
	var resAny any
	resRaw, ok := m.internalMap[key]
	if !ok {
		return 0, eris.New("does not exist in map")
	}
	resAny = resRaw
	res, ok = resAny.(float32)
	if !ok {
		return 0, eris.New("cannot convert to float32")
	}
	return res, nil
}
func (m *MapStorage[K, V]) GetUInt64(_ context.Context, key K) (uint64, error) {
	var res uint64
	var resAny any
	resRaw, ok := m.internalMap[key]
	if !ok {
		return 0, eris.New("does not exist in map")
	}
	resAny = resRaw
	res, ok = resAny.(uint64)
	if !ok {
		return 0, eris.New("cannot convert to uint64")
	}
	return res, nil
}

func (m *MapStorage[K, V]) GetInt64(_ context.Context, key K) (int64, error) {
	var res int64
	var resAny any
	resRaw, ok := m.internalMap[key]
	if !ok {
		return 0, eris.New("does not exist in map")
	}
	resAny = resRaw
	res, ok = resAny.(int64)
	if !ok {
		return 0, eris.New("cannot convert to float")
	}
	return res, nil
}
func (m *MapStorage[K, V]) GetInt(_ context.Context, key K) (int, error) {
	var res int
	var resAny any
	resRaw, ok := m.internalMap[key]
	if !ok {
		return 0, eris.New("does not exist in map")
	}
	resAny = resRaw
	res, ok = resAny.(int)
	if !ok {
		return 0, eris.New("cannot convert to float")
	}
	return res, nil
}
func (m *MapStorage[K, V]) GetBool(_ context.Context, key K) (bool, error) {
	var res bool
	var resAny any
	resRaw, ok := m.internalMap[key]
	if !ok {
		return false, eris.New("does not exist in map")
	}
	resAny = resRaw
	res, ok = resAny.(bool)
	if !ok {
		return false, eris.New("cannot convert to float")
	}
	return res, nil
}
func (m *MapStorage[K, V]) GetBytes(_ context.Context, key K) ([]byte, error) {
	var res []byte
	var resAny any
	resRaw, ok := m.internalMap[key]
	if !ok {
		return nil, eris.New("does not exist in map")
	}
	resAny = resRaw
	res, ok = resAny.([]byte)
	if !ok {
		return nil, eris.New("cannot convert to float")
	}
	return res, nil
}

func (m *MapStorage[K, V]) Set(_ context.Context, key K, value any) error {
	typedValue, ok := value.(V)
	if !ok {
		return eris.New("could not type assert into type")
	}
	m.internalMap[key] = typedValue
	return nil
}

func incrementIfNumeric(value any) (any, bool) {
	switch v := value.(type) {
	case int:
		return v + 1, true
	case int8:
		return v + 1, true
	case int16:
		return v + 1, true
	case int32:
		return v + 1, true
	case int64:
		return v + 1, true
	case uint:
		return v + 1, true
	case uint8:
		return v + 1, true
	case uint16:
		return v + 1, true
	case uint32:
		return v + 1, true
	case uint64:
		return v + 1, true
	case float32:
		return v + 1, true
	case float64:
		return v + 1, true
	case complex64:
		return v + complex(1, 0), true
	case complex128:
		return v + complex(1, 0), true
	default:
		return nil, false
	}
}

func (m *MapStorage[K, V]) Incr(_ context.Context, key K) error {
	value, ok := m.internalMap[key]
	if !ok {
		return eris.New("cannot find element to increment")
	}
	newValue, ok := incrementIfNumeric(value)
	if !ok {
		return eris.New("not numeric, cannot increment")
	}
	value, ok = newValue.(V)
	if !ok {
		return eris.New("cannot type assert back to original value")
	}
	m.internalMap[key] = value
	return nil
}

func decrementIfNumeric(value any) (any, bool) {
	switch v := value.(type) {
	case int:
		return v - 1, true
	case int8:
		return v - 1, true
	case int16:
		return v - 1, true
	case int32:
		return v - 1, true
	case int64:
		return v - 1, true
	case uint:
		return v - 1, true
	case uint8:
		return v - 1, true
	case uint16:
		return v - 1, true
	case uint32:
		return v - 1, true
	case uint64:
		return v - 1, true
	case float32:
		return v - 1, true
	case float64:
		return v - 1, true
	case complex64:
		return v - complex(1, 0), true
	case complex128:
		return v - complex(1, 0), true
	default:
		return nil, false
	}
}

func (m *MapStorage[K, V]) Decr(_ context.Context, key K) error {
	value, ok := m.internalMap[key]
	if !ok {
		return eris.New("cannot find element to increment")
	}
	newValue, ok := decrementIfNumeric(value)
	if !ok {
		return eris.New("not numeric, cannot increment")
	}
	value, ok = newValue.(V)
	if !ok {
		return eris.New("cannot type assert into original type")
	}
	m.internalMap[key] = value
	return nil
}
func (m *MapStorage[K, V]) Delete(_ context.Context, key K) error {
	delete(m.internalMap, key)
	return nil
}

// Does nothing for now
func (m *MapStorage[K, V]) StartTransaction(_ context.Context) (Transaction[K], error) {
	return m, nil
}

// Does nothing for now
func (m *MapStorage[K, V]) EndTransaction(_ context.Context) error {
	return nil
}

// Does nothing.
func (m *MapStorage[K, V]) Close(_ context.Context) error {
	return nil
}
