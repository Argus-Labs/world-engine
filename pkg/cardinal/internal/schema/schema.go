package schema

import (
	"fmt"

	"github.com/rotisserie/eris"
	"github.com/shamaton/msgpack/v3"
)

// Serializable is the interface that all user-defined types (components, commands, events) must implement.
type Serializable interface {
	Name() string
}

// Serialize converts a Serializable to bytes.
// The underlying format is an implementation detail and may change.
func Serialize(s Serializable) ([]byte, error) {
	data, err := msgpack.Marshal(s)
	if err != nil {
		return nil, eris.Wrap(err, "failed to serialize")
	}
	return data, nil
}

// Deserialize converts bytes back into a value.
// The underlying format is an implementation detail and may change.
// The value v must be a pointer to the target type.
func Deserialize(data []byte, v any) (err error) {
	defer func() {
		// TODO: This is a lazy fix because of a bug in shamaton/msgpack/v3 that causes Unmarshal to
		// panic on malformed input. This should be fixed upstream. Unmarshal errors should be returned.
		// For more details, see: https://ampcode.com/threads/T-019c9a82-f628-70f5-ae19-a4300ad53464
		if r := recover(); r != nil {
			err = eris.Wrap(fmt.Errorf("panic: %v", r), "failed to deserialize")
		}
	}()

	if err := msgpack.Unmarshal(data, v); err != nil {
		return eris.Wrap(err, "failed to deserialize")
	}
	return nil
}
