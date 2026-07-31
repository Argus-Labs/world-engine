package testutils

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"io"
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
func (SimpleComponent) UnmarshalWire(b []byte) (any, error) {
	return gobUnmarshal[SimpleComponent](b)
}

func (c ComponentA) MarshalWire() ([]byte, error)             { return gobMarshal(c) }
func (ComponentA) UnmarshalWire(b []byte) (any, error) { return gobUnmarshal[ComponentA](b) }

func (c ComponentB) MarshalWire() ([]byte, error)             { return gobMarshal(c) }
func (ComponentB) UnmarshalWire(b []byte) (any, error) { return gobUnmarshal[ComponentB](b) }

func (c ComponentC) MarshalWire() ([]byte, error)             { return gobMarshal(c) }
func (ComponentC) UnmarshalWire(b []byte) (any, error) { return gobUnmarshal[ComponentC](b) }

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

func (c SimpleSystemEvent) MarshalWire() ([]byte, error) { return gobMarshal(c) }
func (SimpleSystemEvent) UnmarshalWire(b []byte) (any, error) {
	return gobUnmarshal[SimpleSystemEvent](b)
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

// Wire methods for the command fixtures. Real commands get generated proto codecs; these test doubles
// hand-roll encoding/binary and implement schema.Serializable directly (Name + MarshalWire +
// UnmarshalWire). UnmarshalWire returns any — a decode factory that ignores its receiver — so testutils
// needs no import of the engine (which already imports testutils).

func (c SimpleCommand) MarshalWire() ([]byte, error) {
	var b bytes.Buffer
	err := binary.Write(&b, binary.LittleEndian, int64(c.Value))
	return b.Bytes(), err
}

func (SimpleCommand) UnmarshalWire(data []byte) (any, error) {
	var v int64
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &v); err != nil {
		return nil, err
	}
	return SimpleCommand{Value: int(v)}, nil
}

func (c CommandA) MarshalWire() ([]byte, error) {
	var b bytes.Buffer
	err := binary.Write(&b, binary.LittleEndian, c)
	return b.Bytes(), err
}

func (CommandA) UnmarshalWire(data []byte) (any, error) {
	var c CommandA
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &c); err != nil {
		return nil, err
	}
	return c, nil
}

func (c CommandB) MarshalWire() ([]byte, error) {
	var b bytes.Buffer
	if err := binary.Write(&b, binary.LittleEndian, c.ID); err != nil {
		return nil, err
	}
	if err := binary.Write(&b, binary.LittleEndian, c.Enabled); err != nil {
		return nil, err
	}
	// Label is variable-length, so it goes last and consumes the rest on decode.
	if err := binary.Write(&b, binary.LittleEndian, []byte(c.Label)); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (CommandB) UnmarshalWire(data []byte) (any, error) {
	b := bytes.NewReader(data)
	var c CommandB
	if err := binary.Read(b, binary.LittleEndian, &c.ID); err != nil {
		return nil, err
	}
	if err := binary.Read(b, binary.LittleEndian, &c.Enabled); err != nil {
		return nil, err
	}
	label := make([]byte, b.Len())
	if _, err := io.ReadFull(b, label); err != nil {
		return nil, err
	}
	c.Label = string(label)
	return c, nil
}

func (c CommandC) MarshalWire() ([]byte, error) {
	var b bytes.Buffer
	err := binary.Write(&b, binary.LittleEndian, c)
	return b.Bytes(), err
}

func (CommandC) UnmarshalWire(data []byte) (any, error) {
	var c CommandC
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &c); err != nil {
		return nil, err
	}
	return c, nil
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

func (SimpleEvent) UnmarshalWire(b []byte) (any, error) {
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

func (AnotherEvent) UnmarshalWire(b []byte) (any, error) {
	return AnotherEvent{Data: string(b)}, nil
}
