package testutils

// No imports needed since the CreateUnmarshalFunc is now obsolete

// -------------------------------------------------------------------------------------------------
// Test Commands
// -------------------------------------------------------------------------------------------------

// TestCommand is a simple command for testing purposes.
type TestCommand struct {
	Value int `json:"value"`
}

func (TestCommand) Name() string { return "test-command" }

// AnotherTestCommand is another command type for testing multiple command types.
type AnotherTestCommand struct {
	Value int `json:"value"`
}

func (AnotherTestCommand) Name() string { return "another-test-command" }

// SimpleCommand is a basic command with name and payload for marshal testing.
type SimpleCommand struct {
	CommandName string `json:"name"`
	Payload     string `json:"payload"`
}

func (c SimpleCommand) Name() string {
	return c.CommandName
}

// -------------------------------------------------------------------------------------------------
// Test Events
// -------------------------------------------------------------------------------------------------

// SimpleEvent is a basic event with name and payload for marshal testing.
type SimpleEvent struct {
	EventName string `json:"name"`
	Payload   string `json:"payload"`
}

func (e SimpleEvent) Name() string {
	return e.EventName
}

// -------------------------------------------------------------------------------------------------
// Test Payloads
// -------------------------------------------------------------------------------------------------

// TestPayload is a basic struct payload for raw event testing.
type TestPayload struct {
	Key    string `json:"key"`
	Number int    `json:"number"`
}

// CustomPayload is a payload with boolean field for testing custom event kinds.
type CustomPayload struct {
	Custom bool `json:"custom"`
}

// -------------------------------------------------------------------------------------------------
// Factory Functions
// -------------------------------------------------------------------------------------------------

// NewSimpleCommand creates a SimpleCommand with the given name and payload.
func NewSimpleCommand(name, payload string) SimpleCommand {
	return SimpleCommand{CommandName: name, Payload: payload}
}

// NewSimpleEvent creates a SimpleEvent with the given name and payload.
func NewSimpleEvent(name, payload string) SimpleEvent {
	return SimpleEvent{EventName: name, Payload: payload}
}
