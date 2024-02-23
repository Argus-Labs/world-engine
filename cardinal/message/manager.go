package message

import (
	"reflect"
	"slices"

	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/types"
)

type Manager struct {
	registeredMessages       map[string]types.Message
	registeredMessagesByType map[reflect.Type]types.Message
	nextMessageID            types.MessageID
}

func NewManager() *Manager {
	return &Manager{
		registeredMessagesByType: map[reflect.Type]types.Message{},
		registeredMessages:       map[string]types.Message{},
		nextMessageID:            1,
	}
}

func (m *Manager) registerMessagesByName(msgs ...types.Message) error {
	// Iterate through all the messages and check if they are already registered.
	// This is done before registering any of the messages to ensure that all are registered or none of them are.
	msgNames := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		// Check for duplicate message names within the list of messages to be registered
		if slices.Contains(msgNames, msg.Name()) {
			return eris.Errorf("duplicate message %q in slice", msg.Name())
		}

		// Checks if the message is already previously registered.
		// This will terminate the registration of all messages if any of them are already registered.
		if err := m.isMessageNameUnique(msg); err != nil {
			return err
		}

		// If the message is not already registered, add it to the list of message names.
		msgNames = append(msgNames, msg.Name())
	}

	// Iterate through all the messages and register them one by one.
	for _, msg := range msgs {
		// Set EntityID on message
		err := msg.SetID(m.nextMessageID)
		if err != nil {
			return eris.Errorf("failed to set EntityID on message %q", msg.Name())
		}

		// Register message
		m.registeredMessages[msg.Name()] = msg
		m.nextMessageID++
	}

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

// GetMessageByName returns the message with the given name, if it exists.
func (m *Manager) GetMessageByName(name string) (types.Message, bool) {
	msg, ok := m.registeredMessages[name]
	return msg, ok
}

func (m *Manager) GetMessageByType(mType reflect.Type) (types.Message, bool) {
	msg, ok := m.registeredMessagesByType[mType]
	return msg, ok
}

func (m *Manager) registerMessageByType(mType reflect.Type, message types.Message) error {
	_, ok := m.registeredMessagesByType[mType]
	if ok {
		return eris.New("A message of this type has already been registered")
	}
	m.registeredMessagesByType[mType] = message
	return nil
}

// isMessageNameUnique checks if the message name already exist in messages map.
func (m *Manager) isMessageNameUnique(tx types.Message) error {
	_, ok := m.registeredMessages[tx.Name()]
	if ok {
		return eris.Errorf("message %q is already registered", tx.Name())
	}
	return nil
}

// RegisterMessageOnManager registers a single message on a manager. You do not need to provide the message type
// You just need to provide the input and Output parameters
func RegisterMessageOnManager[In any, Out any](manager *Manager, name string, opts ...MessageOption[In, Out]) error {
	msgType := newMessageType[In, Out](name, opts...)
	err := manager.registerMessagesByName(msgType)
	if err != nil {
		return err
	}
	typeValueOfMessageType := reflect.TypeOf(*msgType)
	return manager.registerMessageByType(typeValueOfMessageType, msgType)
}
