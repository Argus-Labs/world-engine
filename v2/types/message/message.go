package message

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"pkg.world.dev/world-engine/cardinal/v2/codec"
	"pkg.world.dev/world-engine/sign"
)

var ErrEVMTypeNotSet = errors.New("EVM type is not set")
var DefaultGroup = "game"

// enforces first/last (or single) alphanumeric character, can contain dash/slash in between.
// does not allow spaces or special characters.
var messageRegexp = regexp.MustCompile(`^[a-zA-Z0-9-]+(\.[a-zA-Z0-9-]+)?$`)

type Message interface {
	Name() string
}

type MessageInternal interface {
	Message
	Group() string
	GetSchema() map[string]any
	Encode(Tx) ([]byte, error)
	Decode(*sign.Transaction) (Tx, error)
}

// MessageType manages a user defined state transition message struct.
type MessageType[Msg Message] struct {
	name  string
	group string
}

var _ Message = &MessageType[Message]{}
var _ MessageInternal = &MessageType[Message]{}

// NewMessageType creates a new message type. It accepts two generic type parameters: the first for the message input,
// which defines the data needed to make a state transition, and the second for the message output, commonly used
// for the results of a state transition. By default, messages will be grouped under the "game" group, however an option
// may be passed in to change this.
func NewMessageType[Msg Message](opts ...Option[Msg]) *MessageType[Msg] {
	var msg Msg

	if !isValidMsg[Msg]() {
		panic(fmt.Sprintf("Invalid MessageType: %q: The In and Out must be both structs", msg.Name()))
	}

	// If Msg.Name() is `<name>`, use the default "game" group.
	var group string
	msgNameParts := strings.Split(msg.Name(), ".")

	if len(msgNameParts) == 1 {
		// If Msg.Name() is `<name>`, use the default "game" group.
		group = DefaultGroup
	} else if len(msgNameParts) == 2 {
		// If Msg.Name() is `<group>.<name>`, use the custom group.
		group = msgNameParts[0]
	} else {
		// If Msg.Name() is `<group>.<name>.<...>`, panic.
		// Technically, this should never happen because we check for this in isValidMsg, but just to be safe.
		panic(fmt.Sprintf("Invalid message name: %q", msg.Name()))
	}

	msgType := MessageType[Msg]{
		name:  msg.Name(),
		group: group,
	}
	for _, opt := range opts {
		opt(&msgType)
	}

	return &msgType
}

func (t *MessageType[Msg]) Name() string {
	return t.name
}

func (t *MessageType[Msg]) Group() string {
	return t.group
}

// Encode encodes the transaction to its JSON representation.
func (t *MessageType[Msg]) Encode(tx Tx) ([]byte, error) {
	return codec.Encode(tx)
}

// Decode decodes the message from the transaction's body.
func (t *MessageType[Msg]) Decode(tx *sign.Transaction) (Tx, error) {
	msg, err := codec.Decode[Msg](tx.Body)
	if err != nil {
		return nil, err
	}
	return txType[Msg]{
		Transaction: tx,
		msg:         msg,
	}, nil
}

// GetSchema returns the schema of the message Msg type.
func (t *MessageType[Msg]) GetSchema() map[string]any {
	var msg Msg
	return getStructSchema(reflect.TypeOf(msg))
}

// -------------------------- Options --------------------------

type Option[Msg Message] func(mt *MessageType[Msg])

// -------------------------- Helpers --------------------------

// isValidMsg checks that Msg.Name() is not empty and adheres to the message name regex.
func isValidMsg[Msg Message]() bool {
	var msg Msg
	msgName := msg.Name()
	if msgName == "" {
		return false
	}
	return messageRegexp.MatchString(msgName)
}

// getStructSchema returns a struct's schema map (key: field name, value: field type OR nested schema map).
// If the field has a json tag, it will be used as the key, otherwise the struct field name will be used.
// It returns nil if the type is not a struct.
func getStructSchema(t reflect.Type) map[string]any {
	// If t is a pointer, dereference it
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	// Terminate if the type is not a struct (after dereferencing)
	if t.Kind() != reflect.Struct {
		return nil
	}

	schema := make(map[string]any)

	// Iterate over all fields in the struct
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldType := field.Type

		// If the field type is a pointer, obtain the type it points to
		if fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}

		// Get field name from json tag or struct field name
		var fieldName string
		if tag := field.Tag.Get("json"); tag != "" {
			fieldName = strings.Split(tag, ",")[0] // Handle cases like `json:"name,omitempty"`
		} else {
			fieldName = field.Name
		}

		switch fieldType.Kind() {
		// If the field is a struct, recursively obtain its schema
		case reflect.Struct:
			schema[fieldName] = getStructSchema(fieldType)

		// If the field is an interface, try to resolve it to a concrete type
		case reflect.Interface:
			// Obtain the backing value of the interface
			// If its backed by nil, treat it as "interface{}" and move on
			concreteValue := reflect.ValueOf(t)
			if concreteValue.IsNil() {
				schema[fieldName] = "interface{}"
				continue
			}

			// Otherwise, figure out the concrete type of the interface
			concreteValueType := concreteValue.Type()
			if concreteValueType.Kind() == reflect.Struct {
				// If the interface is backed by a struct, recursively obtain its schema
				schema[fieldName] = getStructSchema(concreteValueType)
			} else if concreteValueType.Kind() == reflect.Interface || concreteValueType.Kind() == reflect.Pointer {
				// While technically possible (unlikely) for the interface to be backed by another interface/pointer,
				// we don't support recursively dereferencing it since it can lead to infinite loops when we have
				// circular dependencies. Therefore, we will just fallback to "interface{}".
				schema[fieldName] = "interface{}"
			} else {
				// Otherwise, the interface is backed by a primitive type, set value to the type name's string representation
				schema[fieldName] = concreteValueType.String()
			}

		// Otherwise, the field is a primitive type, set value to the type name's string representation
		default:
			schema[fieldName] = fieldType.String()
		}
	}

	return schema
}
