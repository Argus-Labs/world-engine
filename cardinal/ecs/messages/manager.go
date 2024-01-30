package msgs

import (
	"errors"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/types/message"
	"slices"
)

var ErrDuplicateMessageName = errors.New("message names must be unique")

type Manager struct {
	registeredMessages map[string]message.Message
	nextMessageID      message.TypeID
}

func New() *Manager {
	return &Manager{
		registeredMessages: map[string]message.Message{},
		nextMessageID:      1,
	}
}

// RegisterMessages registers the list of message.Messages with the Manager. Returns an error if any of the
// messages have duplicate names.
func (m *Manager) RegisterMessages(msgs ...message.Message) error {
	// We check for duplicate message names within the slice and against the map of registered messages.
	// This ensures that we are registering all messages in the slice, or none of them.
	seenNames := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		// Check for duplicate message names against within the slice
		if slices.Contains(seenNames, msg.Name()) {
			return eris.Wrapf(ErrDuplicateMessageName, "duplicate tx %q", msg.Name())
		}

		// Check for duplicate message names against the map of registere messages
		done, err := m.isNotDuplicate(msg)
		if done {
			return err
		}

		seenNames = append(seenNames, msg.Name())
	}

	// Register all messages
	for _, msg := range msgs {
		err := m.registerMessage(msg)
		if err != nil {
			return eris.Wrapf(err, "failed to register message %q", msg.Name())
		}
	}

	return nil
}

// registerMessage registers a message.Message with the Manager. This should not be used directly.
// Instead, use Manager.RegisterMessages.
func (m *Manager) registerMessage(msg message.Message) error {
	// Sanity check: Check for duplicate message names against the map of registered messages
	_, ok := m.registeredMessages[msg.Name()]
	if ok {
		return eris.Wrapf(ErrDuplicateMessageName, "duplicate tx %q", msg.Name())
	}

	// Set ID on message
	err := msg.SetID(m.nextMessageID)
	if err != nil {
		return err
	}

	// Register message
	m.registeredMessages[msg.Name()] = msg
	m.nextMessageID++

	return nil
}

// IsMessagesRegistered returns true if any messages have been registered with the Manager.
func (m *Manager) IsMessagesRegistered() bool {
	return len(m.registeredMessages) > 0
}

// GetRegisteredMessages returns the list of all registered messages
func (m *Manager) GetRegisteredMessages() []message.Message {
	msgs := make([]message.Message, 0, len(m.registeredMessages))
	for _, msg := range m.registeredMessages {
		msgs = append(msgs, msg)
	}
	return msgs
}

// GetMessage iterates over the all registered messages and returns the message.Message associated with the
// message.TypeID.
func (m *Manager) GetMessage(id message.TypeID) message.Message {
	for _, msg := range m.registeredMessages {
		if id == msg.ID() {
			return msg
		}
	}
	return nil
}

// isNotDuplicate checks for duplicate message names against the map of registered messages.
func (m *Manager) isNotDuplicate(tx message.Message) (bool, error) {
	_, ok := m.registeredMessages[tx.Name()]
	if ok {
		return true, eris.Wrapf(ErrDuplicateMessageName, "duplicate tx %q", tx.Name())
	}
	return false, nil
}
