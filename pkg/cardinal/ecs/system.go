package ecs

// System is a function that contains game logic.
type System[T any] func(state *T) error

// initSystem represents a system that should be run once during world initialization.
type initSystem struct {
	name string       // The name of the system
	fn   func() error // Function that wraps a System
}

// systemConfig holds all configurable options for system registration.
type systemConfig struct {
	// The hook that determines when the system should be executed.
	hook SystemHook
	// Functions that can be applied to the system state fields during initialization.
	modifiers map[systemStateFieldType]func(any) error
}

// newSystemConfig creates a new system config with default values.
func newSystemConfig() systemConfig {
	return systemConfig{
		hook:      Update,
		modifiers: make(map[systemStateFieldType]func(any) error, 0),
	}
}

// SystemOption is a function that configures a SystemConfig.
type SystemOption func(*systemConfig)

// SystemHook defines when a system should be executed in the update cycle.
type SystemHook uint8

const (
	// PreUpdate runs before the main update.
	PreUpdate SystemHook = 0
	// Update runs during the main update phase.
	Update SystemHook = 1
	// PostUpdate runs after the main update.
	PostUpdate SystemHook = 2
	// Init runs once during world initialization.
	Init SystemHook = 3
)

// WithHook returns an option to set the system hook.
func WithHook(hook SystemHook) SystemOption {
	return func(cfg *systemConfig) { cfg.hook = hook }
}

// WithModifier returns an option to set a modifier for a specific field type.
func WithModifier(fieldType systemStateFieldType, fn func(any) error) SystemOption {
	return func(cfg *systemConfig) { cfg.modifiers[fieldType] = fn }
}
