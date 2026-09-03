package schema

import "google.golang.org/protobuf/reflect/protoreflect"

// Serializable is implemented by every user-defined type — command, event, component, and system event
// alike. The generator emits all three methods on each type; a hand-written type that hasn't been
// generated is missing them, so it fails to compile (the LSP flags it) rather than at runtime. One
// interface for all kinds: there is no per-kind codec type and no codec registry.
//
// All methods are value receivers, so the value type satisfies Serializable — the engine never needs a
// *T. UnmarshalWire is a decode factory: it ignores its receiver (which only selects the type) and
// returns the freshly-decoded value. It returns any, not Serializable, so any package can implement it
// without importing this one (no import cycles); callers that know the static type assert the result
// back to it (e.g. decoded.(MoveCommand)).
type Serializable interface {
	Name() string
	// MarshalWire encodes the value. It returns no error, so the generated implementation PANICS on a
	// marshal failure rather than propagating one (see the generator's wireCodec) — it never signals
	// failure by returning nil, and callers must not test for that.
	//
	// Encoding is not infallible: proto.Marshal rejects a proto3 string holding bytes that are not
	// valid UTF-8, which a backend can produce by slicing a string mid-rune or by reading bytes from
	// outside the process. Such a value cannot arrive over the wire (decode rejects it too), so it is
	// always locally made. The panic is unrecovered on the tick's snapshot path.
	MarshalWire() []byte
	UnmarshalWire([]byte) (any, error)
}

// ProtoDescriber supplies protobuf metadata for SDK-generated wire types.
// Debug introspection requires registered types to implement it.
type ProtoDescriber interface {
	ProtoDescriptor() protoreflect.MessageDescriptor
}
