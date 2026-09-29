package schema

import "google.golang.org/protobuf/reflect/protoreflect"

// Serializable is implemented by every user-defined type — command, event, component, and system event
// alike. The generator emits every method on each type; a hand-written type that hasn't been
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

	// SizeWire reports the exact encoded size of the value, and AppendWire writes exactly that many
	// bytes onto b. Neither allocates. Together they are THE encoder: everything that writes a value
	// goes through them, and Marshal below is a thin wrapper rather than a second implementation.
	SizeWire() int
	AppendWire(b []byte) []byte

	UnmarshalWire([]byte) (any, error)
}

// Marshal encodes v into a fresh, exactly-sized buffer.
//
// For callers that hold a Serializable and want a standalone []byte — an outbound command payload, an
// event on the wire. The hot path does NOT use this: the snapshot appends every value into one shared
// buffer, which is the whole reason the interface is a size/append pair.
//
// Generated types also carry a MarshalWire() method with the same body. That method is deliberately
// absent from this interface: nothing in the engine calls it through the interface, and requiring it
// would force every hand-written test double to implement a method that only repeats this line.
func Marshal(v Serializable) []byte {
	return v.AppendWire(make([]byte, 0, v.SizeWire()))
}

// ProtoDescriber supplies protobuf metadata for SDK-generated wire types.
// Debug introspection requires registered types to implement it.
type ProtoDescriber interface {
	ProtoDescriptor() protoreflect.MessageDescriptor
}
