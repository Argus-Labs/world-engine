package types

type Message interface {
	SetID(MessageID) error
	Name() string
	// Group returns the group of the message.
	Group() string
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
}

// MessageID represents a message's EntityID. EntityID's are assigned to messages when they are registered in a World object.
type MessageID int
