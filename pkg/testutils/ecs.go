package testutils

import (
	"bytes"
	"encoding/gob"

	"github.com/vmihailenco/msgpack/v5"
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

func (c SimpleComponent) SizeWire() int              { return len(c.MarshalWire()) }
func (c SimpleComponent) AppendWire(b []byte) []byte { return append(b, c.MarshalWire()...) }

func (c SimpleComponent) MarshalWire() []byte { return GobMarshal(c) }
func (SimpleComponent) UnmarshalWire(b []byte) (any, error) {
	return gobUnmarshal[SimpleComponent](b)
}

func (c ComponentA) SizeWire() int              { return len(c.MarshalWire()) }
func (c ComponentA) AppendWire(b []byte) []byte { return append(b, c.MarshalWire()...) }

func (c ComponentA) MarshalWire() []byte               { return GobMarshal(c) }
func (ComponentA) UnmarshalWire(b []byte) (any, error) { return gobUnmarshal[ComponentA](b) }

func (c ComponentB) SizeWire() int              { return len(c.MarshalWire()) }
func (c ComponentB) AppendWire(b []byte) []byte { return append(b, c.MarshalWire()...) }

func (c ComponentB) MarshalWire() []byte               { return GobMarshal(c) }
func (ComponentB) UnmarshalWire(b []byte) (any, error) { return gobUnmarshal[ComponentB](b) }

func (c ComponentC) SizeWire() int              { return len(c.MarshalWire()) }
func (c ComponentC) AppendWire(b []byte) []byte { return append(b, c.MarshalWire()...) }

func (c ComponentC) MarshalWire() []byte               { return GobMarshal(c) }
func (ComponentC) UnmarshalWire(b []byte) (any, error) { return gobUnmarshal[ComponentC](b) }

// GobMarshal is the gob encoder shared by every gob-backed test double, here and in other fixture
// packages, so there is one definition of what those bytes are.
//
// Doubles derive SizeWire and AppendWire from their own MarshalWire — len() and append() of it —
// rather than from this directly. That costs an encode per size call, which is irrelevant in a
// fixture, and buys the property that actually matters: the three cannot disagree, whatever a given
// double encodes with. Not every double here uses gob; SimpleEvent writes a varint by hand.
func GobMarshal(v any) []byte {
	var buf bytes.Buffer
	mustWrite(gob.NewEncoder(&buf).Encode(v))
	return buf.Bytes()
}

// mustWrite panics on an encoding failure. These are test doubles: a fixture that cannot encode
// itself is a broken fixture, not a condition callers should handle.
func mustWrite(err error) {
	if err != nil {
		panic(err)
	}
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

func (c SimpleSystemEvent) SizeWire() int              { return len(c.MarshalWire()) }
func (c SimpleSystemEvent) AppendWire(b []byte) []byte { return append(b, c.MarshalWire()...) }

func (c SimpleSystemEvent) MarshalWire() []byte { return GobMarshal(c) }
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
// Command fixtures use the same MessagePack payload format as the runtime.

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

// Legacy-named helpers remain for tests that construct wire requests directly. Runtime encoding
// does not call these methods.

func (c SimpleCommand) SizeWire() int              { return len(c.MarshalWire()) }
func (c SimpleCommand) AppendWire(b []byte) []byte { return append(b, c.MarshalWire()...) }

func (c SimpleCommand) MarshalWire() []byte {
	b, err := msgpack.Marshal(c)
	mustWrite(err)
	return b
}

func (SimpleCommand) UnmarshalWire(data []byte) (any, error) {
	var c SimpleCommand
	err := msgpack.Unmarshal(data, &c)
	return c, err
}

func (c CommandA) SizeWire() int              { return len(c.MarshalWire()) }
func (c CommandA) AppendWire(b []byte) []byte { return append(b, c.MarshalWire()...) }

func (c CommandA) MarshalWire() []byte {
	b, err := msgpack.Marshal(c)
	mustWrite(err)
	return b
}

func (CommandA) UnmarshalWire(data []byte) (any, error) {
	var c CommandA
	err := msgpack.Unmarshal(data, &c)
	return c, err
}

func (c CommandB) SizeWire() int              { return len(c.MarshalWire()) }
func (c CommandB) AppendWire(b []byte) []byte { return append(b, c.MarshalWire()...) }

func (c CommandB) MarshalWire() []byte {
	b, err := msgpack.Marshal(c)
	mustWrite(err)
	return b
}

func (CommandB) UnmarshalWire(data []byte) (any, error) {
	var c CommandB
	err := msgpack.Unmarshal(data, &c)
	return c, err
}

func (c CommandC) SizeWire() int              { return len(c.MarshalWire()) }
func (c CommandC) AppendWire(b []byte) []byte { return append(b, c.MarshalWire()...) }

func (c CommandC) MarshalWire() []byte {
	b, err := msgpack.Marshal(c)
	mustWrite(err)
	return b
}

func (CommandC) UnmarshalWire(data []byte) (any, error) {
	var c CommandC
	err := msgpack.Unmarshal(data, &c)
	return c, err
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

// These helpers encode and decode MessagePack for tests acting as event clients.
func (s SimpleEvent) SizeWire() int              { return len(s.MarshalWire()) }
func (s SimpleEvent) AppendWire(b []byte) []byte { return append(b, s.MarshalWire()...) }

func (s SimpleEvent) MarshalWire() []byte {
	b, err := msgpack.Marshal(s)
	mustWrite(err)
	return b
}

func (SimpleEvent) UnmarshalWire(b []byte) (any, error) {
	var s SimpleEvent
	err := msgpack.Unmarshal(b, &s)
	return s, err
}

type AnotherEvent struct {
	Data string
}

func (AnotherEvent) Name() string {
	return "another_event"
}

// These helpers encode and decode MessagePack for tests acting as event clients.
func (e AnotherEvent) SizeWire() int              { return len(e.MarshalWire()) }
func (e AnotherEvent) AppendWire(b []byte) []byte { return append(b, e.MarshalWire()...) }

func (e AnotherEvent) MarshalWire() []byte {
	b, err := msgpack.Marshal(e)
	mustWrite(err)
	return b
}

func (AnotherEvent) UnmarshalWire(b []byte) (any, error) {
	var e AnotherEvent
	err := msgpack.Unmarshal(b, &e)
	return e, err
}
