package testutils

import (
	"fmt"
	"reflect"

	"pkg.world.dev/world-engine/cardinal/message"
)

func isStruct[T any]() bool {
	var in T
	inType := reflect.TypeOf(in)
	inKind := inType.Kind()
	return (inKind == reflect.Pointer &&
		inType.Elem().Kind() == reflect.Struct) ||
		inKind == reflect.Struct
}

// NewMessageType creates a new message type. It accepts two generic type parameters: the first for the message input,
// which defines the data needed to make a state transition, and the second for the message output, commonly used
// for the results of a state transition. This function is a duplicate of a private function that lives in the message package.
// a testutils version of the function exists here in order to allow the explicit instantiation of MessageType for tests.
func NewMessageType[In, Out any](
	name string,
	opts ...message.MessageOption[In, Out],
) *message.MessageType[In, Out] {
	if name == "" {
		panic("cannot create message without name")
	}
	if !isStruct[In]() || !isStruct[Out]() {
		panic(fmt.Sprintf("Invalid MessageType: %s: The In and Out must be both structs", name))
	}
	msg := &message.MessageType[In, Out]{}
	msg.SetName(name)
	msg.SetGroup("game")
	for _, opt := range opts {
		opt(msg)
	}
	return msg
}
