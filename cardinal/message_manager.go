package cardinal

import (
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/types/message"
	"slices"
)

type MessageManager struct {
	registeredMessages map[string]message.Message
	nextMessageID      message.TypeID
}

func NewMessageManager() *MessageManager {
	return &MessageManager{
		registeredMessages: map[string]message.Message{},
		nextMessageID:      1,
	}
}

// RegisterMessages registers multiple messages with the message manager
// There can only be one message iwuth a given name, which is declared by the user by implementing the Name() method.
// If there is a duplicate message name, an error will be returned and none of the messages will be registered.
func (m *MessageManager) RegisterMessages(msgs ...message.Message) error {
	// Iterate through all the messages and check if they are already registered.
	// This is done before registering any of the messages to ensure that all are registered or none of them are.
	msgNames := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		// Check for duplicate message names within the list of messages to be registered
		if slices.Contains(msgNames, msg.Name()) {
			return eris.Errorf("duplicate message %q in slice", msg.Name())
		}

		// Checks if the message is already previously registered.
		// This will terminate the registration of all systems if any of them are already registered.
		if err := m.isNotDuplicate(msg); err != nil {
			return err
		}

		// If the message is not already registered, add it to the list of message names.
		msgNames = append(msgNames, msg.Name())
	}

	// Iterate through all the systems and register them one by one.
	for _, msg := range msgs {
		// Set ID on message
		err := msg.SetID(m.nextMessageID)
		if err != nil {
			return eris.Errorf("failed to set ID on message %q", msg.Name())
		}

		// Register message
		m.registeredMessages[msg.Name()] = msg
		m.nextMessageID++
	}

	return nil
}

// IsMessagesRegistered returns true if any messages have been registered with the MessageManager.
func (m *MessageManager) IsMessagesRegistered() bool {
	return len(m.registeredMessages) > 0
}

// GetRegisteredMessages returns the list of all registered messages
func (m *MessageManager) GetRegisteredMessages() []message.Message {
	msgs := make([]message.Message, 0, len(m.registeredMessages))
	for _, msg := range m.registeredMessages {
		msgs = append(msgs, msg)
	}
	return msgs
}

// GetMessage iterates over the all registered messages and returns the message.Message associated with the
// message.TypeID.
func (m *MessageManager) GetMessage(id message.TypeID) message.Message {
	for _, msg := range m.registeredMessages {
		if id == msg.ID() {
			return msg
		}
	}
	return nil
}

// isNotDuplicate checks if the message name already exist in messages map.
func (m *MessageManager) isNotDuplicate(tx message.Message) error {
	_, ok := m.registeredMessages[tx.Name()]
	if ok {
		return eris.Errorf("message %q is already registered", tx.Name())
	}
	return nil
}
