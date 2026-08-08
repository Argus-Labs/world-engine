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
	MarshalWire() ([]byte, error)
	UnmarshalWire([]byte) (any, error)
}

// Introspectable is implemented by SDK-generated wire types. It is deliberately separate from
// Serializable: tests and integrations may use a different wire codec, while editor introspection is
// specifically metadata for generated protobuf-backed types.
type Introspectable interface {
	FormSchema() []byte
	ProtoDescriptor() protoreflect.MessageDescriptor
}
