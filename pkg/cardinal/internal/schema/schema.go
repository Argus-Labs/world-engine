package schema

import (
	"github.com/rotisserie/eris"
	"github.com/shamaton/msgpack/v3"
)

// Serializable is the interface that all user-defined types (components, commands, events) must implement.
type Serializable interface {
	Name() string
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
