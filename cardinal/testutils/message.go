package testutils

import (
	"fmt"
	"reflect"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
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
// for the results of a state transition. This function is a duplicate of a private function that lives in the
// message package. a testutils version of the function exists here in order to allow the explicit instantiation
// of MessageType for tests.
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

func GetMessage[In any, Out any](wCtx engine.Context) (*message.MessageType[In, Out], error) {
	var msg message.MessageType[In, Out]
	msgType := reflect.TypeOf(msg)
	tempRes, ok := wCtx.GetMessageByType(msgType)
	if !ok {
		return &msg, eris.Errorf("Could not find %s, Message may not be registered.", msg.Name())
	}
	var _ types.Message = &msg
	res, ok := tempRes.(*message.MessageType[In, Out])
	if !ok {
		return &msg, eris.New("wrong type")
	}
	return res, nil
}
