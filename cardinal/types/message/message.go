package message

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	ethereumAbi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/abi"
	"pkg.world.dev/world-engine/cardinal/codec"
)

var ErrEVMTypeNotSet = errors.New("EVM type is not set")
var DefaultGroup = "game"

// enforces first/last (or single) alphanumeric character, can contain dash/slash in between.
// does not allow spaces or special characters.
var messageRegexp = regexp.MustCompile(`^[a-zA-Z0-9-]+(\.[a-zA-Z0-9-]+)?$`)

type Message interface {
	Name() string
}

type MessageType interface {
	Message
	Group() string
	GetInFieldInformation() map[string]any
	IsEVMCompatible() bool
	Encode(any) ([]byte, error)
	Decode([]byte) (Message, error)
	DecodeEVMBytes([]byte) (Message, error)
	ABIEncode(any) ([]byte, error)
}

// messageType manages a user defined state transition message struct.
type messageType[Msg Message] struct {
	msgType   reflect.Type
	name      string
	group     string
	inEVMType *ethereumAbi.Type
}

var _ Message = &messageType[Message]{}
var _ MessageType = &messageType[Message]{}

// NewMessageType creates a new message type. It accepts two generic type parameters: the first for the message input,
// which defines the data needed to make a state transition, and the second for the message output, commonly used
// for the results of a state transition. By default, messages will be grouped under the "game" group, however an option
// may be passed in to change this.
func NewMessageType[Msg Message](opts ...Option[Msg]) MessageType {
	var msg Msg
	if !isStruct[Msg]() {
		panic(fmt.Sprintf("Invalid MessageType: %q: The In and Out must be both structs", msg.Name()))
	}

	if !isValidMessageText(msg.Name()) {
		panic(fmt.Sprintf("Invalid MessageType: %q: message name must only contain alphanumerics, "+
			"dashes (-). Must also start/end with an alphanumeric.", msg.Name()))
	}

	var group string
	msgNameParts := strings.Split(msg.Name(), ".")
	if len(msgNameParts) == 1 {
		group = DefaultGroup
	} else if len(msgNameParts) == 2 {
		group = msgNameParts[0]
	} else {
		panic(fmt.Sprintf("Invalid message name: %q", msg.Name()))
	}

	msgType := messageType[Msg]{
		name:  msg.Name(),
		group: group,
	}
	for _, opt := range opts {
		opt(&msgType)
	}

	return &msgType
}

func (t *messageType[Msg]) Name() string {
	return t.name
}

func (t *messageType[Msg]) Group() string {
	return t.group
}

func (t *messageType[Msg]) IsEVMCompatible() bool {
	return t.inEVMType != nil
}

func (t *messageType[Msg]) Encode(a any) ([]byte, error) {
	return codec.Encode(a)
}

func (t *messageType[Msg]) Decode(bytes []byte) (Message, error) {
	return codec.Decode[Msg](bytes)
}

// ABIEncode encodes the input to the message's matching evm type. If the input is not either of the message's
// evm types, an error is returned.
func (t *messageType[Msg]) ABIEncode(v any) ([]byte, error) {
	if t.inEVMType == nil {
		return nil, eris.Wrap(ErrEVMTypeNotSet, "")
	}

	var args ethereumAbi.Arguments
	var input any
	
	switch in := v.(type) {
	case Msg:
		input = in
		args = ethereumAbi.Arguments{{Type: *t.inEVMType}}
	default:
		return nil, eris.Errorf("expectedResult input to be of type %T, got %T", new(Msg), v)
	}

	return args.Pack(input)
}

// DecodeEVMBytes decodes abi encoded solidity structs into the message's "In" type.
func (t *messageType[Msg]) DecodeEVMBytes(bz []byte) (Message, error) {
	if t.inEVMType == nil {
		return nil, ErrEVMTypeNotSet
	}
	args := ethereumAbi.Arguments{{Type: *t.inEVMType}}
	unpacked, err := args.Unpack(bz)
	err = eris.Wrap(err, "")
	if err != nil {
		return nil, err
	}
	if len(unpacked) < 1 {
		return nil, eris.Errorf("error decoding EVM bytes: no values could be unpacked into the abi type")
	}
	input, err := abi.SerdeInto[Msg](unpacked[0])
	if err != nil {
		return nil, err
	}
	return input, nil
}

// GetInFieldInformation returns a map of the fields of the message's "In" type and it's field types.
func (t *messageType[Msg]) GetInFieldInformation() map[string]any {
	return getFieldInformation(reflect.TypeOf(new(Msg)).Elem())
}

// -------------------------- Options --------------------------

type Option[Msg Message] func(mt *messageType[Msg])

func WithEVMSupport[Msg Message]() Option[Msg] {
	return func(msg *messageType[Msg]) {
		var in Msg
		var err error
		msg.inEVMType, err = abi.GenerateABIType(in)
		if err != nil {
			panic(err)
		}
	}
}

// -------------------------- Helpers --------------------------

func isStruct[T any]() bool {
	var in T
	inType := reflect.TypeOf(in)
	inKind := inType.Kind()
	return (inKind == reflect.Pointer &&
		inType.Elem().Kind() == reflect.Struct) ||
		inKind == reflect.Struct
}

// isValidMessageText checks that a messages name or group adheres to the regexp.
func isValidMessageText(txt string) bool {
	return messageRegexp.MatchString(txt)
}

func getFieldInformation(t reflect.Type) map[string]any {
	if t.Kind() != reflect.Struct {
		return nil
	}

	fieldMap := make(map[string]any)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldName := field.Name

		// Check if the field has a json tag
		if tag := field.Tag.Get("json"); tag != "" {
			fieldName = tag
		}

		if field.Type.Kind() == reflect.Struct {
			fieldMap[fieldName] = getFieldInformation(field.Type)
		} else {
			fieldMap[fieldName] = field.Type.String()
		}
	}

	return fieldMap
}
