package schema

import (
	"encoding"
	"fmt"

	"github.com/rotisserie/eris"
	"github.com/shamaton/msgpack/v3"
)

// Serializable is the interface that all user-defined types (components, commands, events) must implement.
type Serializable interface {
	Name() string
}

// MarshalCommand serializes a command for transport across a shard or process boundary. The engine cannot know a
// user-defined command's shape, so each command supplies its own binary encoding via the standard
// encoding.BinaryMarshaler. A command that does not implement it cannot be transported and returns an error.
func MarshalCommand(s Serializable) ([]byte, error) {
	m, ok := s.(encoding.BinaryMarshaler)
	if !ok {
		return nil, eris.Errorf("command %q has no wire layer (run the generator)", s.Name())
	}
	return m.MarshalBinary()
}

// UnmarshalCommand reconstructs a command from its transported bytes into v (a pointer to the command type)
// via the standard encoding.BinaryUnmarshaler.
func UnmarshalCommand(v any, data []byte) error {
	u, ok := v.(encoding.BinaryUnmarshaler)
	if !ok {
		name := "?"
		if s, ok := v.(Serializable); ok {
			name = s.Name()
		}
		return eris.Errorf("command %q has no wire layer (run the generator)", name)
	}
	return u.UnmarshalBinary(data)
}

// Serialize converts a Serializable (component/event) to bytes via msgpack.
// The underlying format is an implementation detail and may change.
func Serialize(s Serializable) ([]byte, error) {
	data, err := msgpack.Marshal(s)
	if err != nil {
		return nil, eris.Wrap(err, "failed to serialize")
	}
	return data, nil
}

// Deserialize converts bytes back into a value (component/event) via msgpack.
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
