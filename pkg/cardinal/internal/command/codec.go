package command

import "github.com/rotisserie/eris"

// Codec encodes and decodes a command payload to and from its wire bytes. Each command type has its
// own codec (generated from its schema); the engine does not know a command's shape, so it looks the
// codec up by command name. Unmarshal returns a fresh payload rather than mutating a receiver, so a
// codec implementation needs no pointer receivers.
type Codec interface {
	Marshal(Payload) ([]byte, error)
	Unmarshal([]byte) (Payload, error)
}

// codecs maps a command name to its wire codec. It is populated once at init time by generated code
// (command package imported wherever the command is used), then only read, so it needs no locking.
//
//nolint:gochecknoglobals // command codec registry: set once at init, read-only thereafter
var codecs = map[string]Codec{}

// RegisterCodec registers the wire codec for a command name. Generated code calls this from init().
func RegisterCodec(name string, c Codec) {
	codecs[name] = c
}

// HasCodec reports whether a codec is registered for the command name.
func HasCodec(name string) bool {
	_, ok := codecs[name]
	return ok
}

// Marshal encodes a command payload using its registered codec.
func Marshal(p Payload) ([]byte, error) {
	c, ok := codecs[p.Name()]
	if !ok {
		return nil, eris.Errorf("command %q has no registered codec (run the generator)", p.Name())
	}
	return c.Marshal(p)
}

// unmarshal decodes wire bytes for the named command using its registered codec.
func unmarshal(name string, data []byte) (Payload, error) {
	c, ok := codecs[name]
	if !ok {
		return nil, eris.Errorf("command %q has no registered codec (run the generator)", name)
	}
	return c.Unmarshal(data)
}
