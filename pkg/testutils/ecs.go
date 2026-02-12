package testutils

// -------------------------------------------------------------------------------------------------
// Components
// -------------------------------------------------------------------------------------------------

type SimpleComponent struct {
	Value int
}

func (SimpleComponent) Name() string {
	return "simple_component"
}

type ComponentA struct {
	X, Y, Z float64
}

func (ComponentA) Name() string {
	return "component_a"
}

type ComponentB struct {
	ID      uint64
	Label   string
	Enabled bool
}

func (ComponentB) Name() string {
	return "component_b"
}

type ComponentC struct {
	Values  [8]int32
	Counter uint16
}

func (ComponentC) Name() string {
	return "component_c"
}

// ComponentMixed is a comprehensive test component with all common field types.
// Used to verify MessagePack correctly handles the full range of Go types.
type ComponentMixed struct {
	// Integer types
	Int8Val   int8
	Int16Val  int16
	Int32Val  int32
	Int64Val  int64
	Uint8Val  uint8
	Uint16Val uint16
	Uint32Val uint32
	Uint64Val uint64 // Critical: values > 2^53-1 lose precision in JSON

	// Floating point
	Float32Val float32
	Float64Val float64

	// String and bool
	StringVal string
	BoolVal   bool

	// Slice and array
	IntSlice   []int
	ByteSlice  []byte
	FloatArray [3]float64

	// Nested struct
	Nested NestedData

	// Map
	Metadata map[string]int
}

type NestedData struct {
	ID    uint64
	Name  string
	Score float64
}

func (ComponentMixed) Name() string {
	return "component_mixed"
}

// -------------------------------------------------------------------------------------------------
// System events
// -------------------------------------------------------------------------------------------------

type SimpleSystemEvent struct {
	Value int
}

func (SimpleSystemEvent) Name() string {
	return "simple_system_event"
}

type SystemEventA struct {
	X, Y, Z float64
}

func (SystemEventA) Name() string {
	return "system_event_a"
}

type SystemEventB struct {
	ID      uint64
	Label   string
	Enabled bool
}

func (SystemEventB) Name() string {
	return "system_event_b"
}

type SystemEventC struct {
	Values  [8]int32
	Counter uint16
}

func (SystemEventC) Name() string {
	return "system_event_c"
}

// -------------------------------------------------------------------------------------------------
// Commands
// -------------------------------------------------------------------------------------------------

type SimpleCommand struct {
	Value int
}

func (SimpleCommand) Name() string {
	return "simple_command"
}

type CommandA struct {
	X, Y, Z float64
}

func (CommandA) Name() string {
	return "command_a"
}

type CommandB struct {
	ID      uint64
	Label   string
	Enabled bool
}

func (CommandB) Name() string {
	return "command_b"
}

type CommandC struct {
	Values  [8]int32
	Counter uint16
}

func (CommandC) Name() string {
	return "command_c"
}

// CommandUint64 is a test command with uint64 fields for precision testing.
type CommandUint64 struct {
	Amount    uint64
	EntityID  uint64
	Timestamp int64
}

func (CommandUint64) Name() string {
	return "command_uint64"
}

// -------------------------------------------------------------------------------------------------
// Events
// -------------------------------------------------------------------------------------------------

type SimpleEvent struct {
	Value int
}

func (SimpleEvent) Name() string {
	return "simple_event"
}

type AnotherEvent struct {
	Data string
}

func (AnotherEvent) Name() string {
	return "another_event"
}
