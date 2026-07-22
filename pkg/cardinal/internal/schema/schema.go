package schema

// Serializable is the interface that all user-defined types (components, commands, events) must implement.
type Serializable interface {
	Name() string
}
