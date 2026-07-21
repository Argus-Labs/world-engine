package command

import (
	"sync/atomic"

	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Codec encodes and decodes a command payload to and from its wire bytes. Each command type has its
// own codec (generated from its schema); the engine does not know a command's shape, so it looks the
// codec up by command name. Unmarshal returns a fresh payload rather than mutating a receiver, so a
// codec implementation needs no pointer receivers.
type Codec interface {
	Marshal(Payload) ([]byte, error)
	Unmarshal([]byte) (Payload, error)
}

type registeredCodec struct {
	codec      Codec
	descriptor protoreflect.MessageDescriptor
}

// codecs maps a command name to its wire codec. It is populated once at init time by generated code
// (command package imported wherever the command is used), then only read, so it needs no locking.
//
//nolint:gochecknoglobals // command codec registry: set once at init, read-only thereafter
var codecs = map[string]registeredCodec{}

// sealed freezes the registry once the world starts. After that the map is immutable, so the hot-path
// reads in Marshal/unmarshal stay lock-free; a register attempt after sealing is a programming error.
//
//nolint:gochecknoglobals // paired with codecs; write-once at StartGame
var sealed atomic.Bool

// Seal freezes the codec registry. cardinal calls it when a world starts. Codecs must be registered
// from generated init() (which runs before any StartGame); registering afterwards would be an
// unrecoverable concurrent-map write against the command hot path, so a post-seal register panics
// deterministically instead of racing.
func Seal() {
	sealed.Store(true)
}

// RegisterCodec registers the wire codec for a command name. Generated code calls this from init().
// A name may be registered only once: a duplicate means two generated files claim the same wire name
// (e.g. a stale commands_wire.gen.go left behind after a command moved packages), so we fail loudly at
// startup rather than let a last-wins map write silently shadow the correct codec. Registration must
// happen before the world starts (Seal); a later call panics rather than racing the hot-path reads.
func RegisterCodec(name string, codec Codec, descriptor protoreflect.MessageDescriptor) {
	if sealed.Load() {
		panic(eris.Errorf(
			"command codec registry is sealed: register %q from generated init(), not after the world starts",
			name,
		))
	}
	if _, exists := codecs[name]; exists {
		panic(eris.Errorf("command %q already has a registered codec (duplicate or stale generated code)", name))
	}
	if descriptor == nil {
		panic(eris.Errorf(
			"command %q codec has no protobuf message descriptor (regenerate it with world sdk generate)",
			name,
		))
	}
	codecs[name] = registeredCodec{codec: codec, descriptor: descriptor}
}

// HasCodec reports whether a codec is registered for the command name.
func HasCodec(name string) bool {
	_, ok := codecs[name]
	return ok
}

// MessageDescriptor returns the exact protobuf message descriptor used by the named command codec.
func MessageDescriptor(name string) protoreflect.MessageDescriptor {
	c, ok := codecs[name]
	if !ok {
		return nil
	}
	return c.descriptor
}

// Marshal encodes a command payload using its registered codec.
func Marshal(p Payload) ([]byte, error) {
	c, ok := codecs[p.Name()]
	if !ok {
		return nil, eris.Errorf("command %q has no registered codec (run the generator)", p.Name())
	}
	return c.codec.Marshal(p)
}

// unmarshal decodes wire bytes for the named command using its registered codec.
func unmarshal(name string, data []byte) (Payload, error) {
	c, ok := codecs[name]
	if !ok {
		return nil, eris.Errorf("command %q has no registered codec (run the generator)", name)
	}
	return c.codec.Unmarshal(data)
}
