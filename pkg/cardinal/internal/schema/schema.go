package schema

import (
	"fmt"
	"reflect"

	"github.com/rotisserie/eris"
	"github.com/vmihailenco/msgpack/v5"
)

// Serializable is the interface all user-defined payload types must implement.
type Serializable interface {
	Name() string
}

// Serialize encodes a payload as MessagePack.
func Serialize(s Serializable) ([]byte, error) {
	data, err := msgpack.Marshal(s)
	if err != nil {
		return nil, eris.Wrap(err, "failed to serialize")
	}
	return data, nil
}

// Deserialize replaces the target with a freshly decoded value. The target must be a non-nil
// pointer. Missing fields start at zero; failed decoding leaves the target unchanged.
func Deserialize(data []byte, v any) (err error) {
	defer func() {
		// Keep malformed input and custom decoder panics at the serialization boundary.
		if r := recover(); r != nil {
			err = eris.Wrap(fmt.Errorf("panic: %v", r), "failed to deserialize")
		}
	}()

	target := reflect.ValueOf(v)
	if target.Kind() != reflect.Pointer || target.IsNil() {
		return eris.New("failed to deserialize: target must be a non-nil pointer")
	}
	fresh := reflect.New(target.Elem().Type())
	if err := msgpack.Unmarshal(data, fresh.Interface()); err != nil {
		return eris.Wrap(err, "failed to deserialize")
	}
	target.Elem().Set(fresh.Elem())
	return nil
}
