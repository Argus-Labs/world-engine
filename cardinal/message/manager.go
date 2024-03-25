package message

import (
	"errors"
	"reflect"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/types"
)

type Manager struct {
	// registeredMessages maps message FullNames to a types.Message.
	registeredMessages       map[string]types.Message
	registeredMessagesByType map[reflect.Type]types.Message
	nextMessageID            types.MessageID
}

func NewManager() *Manager {
	return &Manager{
		registeredMessages:       map[string]types.Message{},
		registeredMessagesByType: map[reflect.Type]types.Message{},
		nextMessageID:            1,
	}
}

func (m *Manager) RegisterMessage(msgType types.Message, msgReflectType reflect.Type) error {
	fullName := msgType.FullName()
	// Checks if the message is already previously registered.
	if err := errors.Join(m.isMessageFullNameUnique(fullName), m.isMessageTypeUnique(msgReflectType)); err != nil {
		return err
	}

	// Set the message ID.
	// TODO(scott): we should probably deprecate this and just decide whether we want to use fullName or ID.
	err := msgType.SetID(m.nextMessageID)
	if err != nil {
		return eris.Errorf("failed to set id on message %q", msgType.Name())
	}

	m.registeredMessages[fullName] = msgType
	m.registeredMessagesByType[msgReflectType] = msgType
	m.nextMessageID++

	return nil
}

// GetRegisteredMessages returns the list of all registered messages
func (m *Manager) GetRegisteredMessages() []types.Message {
	msgs := make([]types.Message, 0, len(m.registeredMessages))
	for _, msg := range m.registeredMessages {
		msgs = append(msgs, msg)
	}
	return msgs
}

// GetMessageByID iterates over the all registered messages and returns the types.Message associated with the
// MessageID.
func (m *Manager) GetMessageByID(id types.MessageID) types.Message {
	for _, msg := range m.registeredMessages {
		if id == msg.ID() {
			return msg
		}
	}
	return nil
}

// GetMessageByFullName returns the message with the given full name, if it exists.
func (m *Manager) GetMessageByFullName(fullName string) (types.Message, bool) {
	msg, ok := m.registeredMessages[fullName]
	return msg, ok
}

func (m *Manager) GetMessageByType(mType reflect.Type) (types.Message, bool) {
	msg, ok := m.registeredMessagesByType[mType]
	return msg, ok
}

// isMessageFullNameUnique checks if the message name already exist in messages map.
func (m *Manager) isMessageFullNameUnique(fullName string) error {
	_, ok := m.registeredMessages[fullName]
	if ok {
		return eris.Errorf("message %q is already registered", fullName)
	}
	return nil
}

// isMessageTypeUnique checks if the message type name already exist in messages map.
func (m *Manager) isMessageTypeUnique(msgReflectType reflect.Type) error {
	_, ok := m.registeredMessagesByType[msgReflectType]
	if ok {
		return eris.Errorf("message type %q is already registered", msgReflectType)
	}
	return nil
}
