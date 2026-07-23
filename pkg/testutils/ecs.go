package testutils

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
)

// Test components use gob for their wire codec — real components get generated proto codecs, but test
// doubles only need something that round-trips. gob (not json) so no struct tags are needed: json tags
// would rename the fields the search engine resolves by (it keys on the Go field name).

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

func (c SimpleComponent) MarshalWire() ([]byte, error) { return gobMarshal(c) }
func (SimpleComponent) UnmarshalWire(b []byte) (SimpleComponent, error) {
	return gobUnmarshal[SimpleComponent](b)
}

func (c ComponentA) MarshalWire() ([]byte, error)             { return gobMarshal(c) }
func (ComponentA) UnmarshalWire(b []byte) (ComponentA, error) { return gobUnmarshal[ComponentA](b) }

func (c ComponentB) MarshalWire() ([]byte, error)             { return gobMarshal(c) }
func (ComponentB) UnmarshalWire(b []byte) (ComponentB, error) { return gobUnmarshal[ComponentB](b) }

func (c ComponentC) MarshalWire() ([]byte, error)             { return gobMarshal(c) }
func (ComponentC) UnmarshalWire(b []byte) (ComponentC, error) { return gobUnmarshal[ComponentC](b) }

func gobMarshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gobUnmarshal[T any](b []byte) (T, error) {
	var v T
	err := gob.NewDecoder(bytes.NewReader(b)).Decode(&v)
	return v, err
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
// Command fixtures are plain value structs (just Name); their wire codec lives in the commandtest
// package, which registers it with the command package the same way the generator does for real
// commands. testutils can't import the internal command package, so the codec can't live here.

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

// -------------------------------------------------------------------------------------------------
// Events
// -------------------------------------------------------------------------------------------------

type SimpleEvent struct {
	Value int
}

func (SimpleEvent) Name() string {
	return "simple_event"
}

// MarshalWire / UnmarshalWire are a test double for generated event wire code — the engine requires the
// wire codec (no msgpack fallback). Deliberately an explicit encoding, not a serialization library, so
// testutils stays free of any wire-format dependency.
func (s SimpleEvent) MarshalWire() ([]byte, error) {
	return binary.AppendVarint(nil, int64(s.Value)), nil
}

func (SimpleEvent) UnmarshalWire(b []byte) (SimpleEvent, error) {
	v, n := binary.Varint(b)
	if n <= 0 {
		return SimpleEvent{}, errors.New("SimpleEvent: malformed wire bytes")
	}
	return SimpleEvent{Value: int(v)}, nil
}

type AnotherEvent struct {
	Data string
}

func (AnotherEvent) Name() string {
	return "another_event"
}

// MarshalWire is a test double for generated event wire code (explicit encoding, no serialization lib).
func (e AnotherEvent) MarshalWire() ([]byte, error) {
	return []byte(e.Data), nil
}
