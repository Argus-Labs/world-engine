package types

type Message interface {
	SetID(MessageID) error
	Name() string
	// Group returns the group of the message.
	Group() string
	// FullName returns the fully qualified name of the message. It is formed by <group_name>.<message_name>.
	// example: persona.create-persona
	FullName() string
	ID() MessageID
	Encode(any) ([]byte, error)
	Decode([]byte) (any, error)
	// DecodeEVMBytes decodes ABI encoded bytes into the message's input type.
	DecodeEVMBytes([]byte) (any, error)
	// ABIEncode encodes the given type in ABI encoding, given that the input is the message type's input or output
	// type.
	ABIEncode(any) ([]byte, error)
	// IsEVMCompatible reports if this message can be sent from the EVM.
	IsEVMCompatible() bool

	// GetInFieldInformation returns a map of the fields of the message's "In" type and it's field types.
	GetInFieldInformation() map[string]any
}

// MessageID represents a message's id.
// They are assigned to messages when they are registered in a World object.
type MessageID int
