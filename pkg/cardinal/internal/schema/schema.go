package schema

import (
	"github.com/rotisserie/eris"
	"github.com/shamaton/msgpack/v3"
)

// Serializable is the interface that all user-defined types (components, commands, events) must implement.
type Serializable interface {
	Name() string
}

// ToMsgpack converts a Serializable to MessagePack bytes.
// MessagePack preserves uint64 precision unlike JSON which uses float64.
func ToMsgpack(s Serializable) ([]byte, error) {
	data, err := msgpack.Marshal(s)
	if err != nil {
		return nil, eris.Wrap(err, "failed to marshal to msgpack")
	}
	return data, nil
}
