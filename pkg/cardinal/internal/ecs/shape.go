package ecs

import (
	"hash/fnv"
	"io"
	"reflect"

	"google.golang.org/protobuf/encoding/protowire"
)

// A shape is what a component's values look like on the wire: its field names, their wire types and
// the order they are declared in. Two structs with the same shape encode and decode each other's
// bytes; two with different shapes do not.
//
// The snapshot stores one shape hash per component name. That is what lets a later build tell a
// value it can read from one it has to migrate, and it is the only trigger there is: a save written
// by the build reading it carries matching hashes everywhere, so a restart migrates nothing.
//
// What the hash deliberately does not cover is meaning. Metres becoming centimetres leaves every
// byte and every hash identical. Whether stored values still mean what the code thinks is the
// developer's call, not something the engine can check.
//
// The shape is read from the Go struct rather than from a protobuf descriptor, because both have
// to work: generated components carry a descriptor, hand-written ones and retired shapes do not.
// The generator numbers fields in declaration order, so the Go struct carries the same information
// the descriptor would.
const unknownShape uint64 = 0

// shapeHash returns the wire shape fingerprint of a component value. A type that is not a struct
// has no fields to describe and hashes to unknownShape.
func shapeHash(v any) uint64 {
	t := reflect.TypeOf(v)
	if t == nil || t.Kind() != reflect.Struct {
		return unknownShape
	}

	hash := fnv.New64a()
	writeShape(hash, t, 0)
	sum := hash.Sum64()
	if sum == unknownShape {
		// One value in 2^64 collides with the sentinel. Moving it off keeps "zero means unknown"
		// true without weakening anything else.
		return 1
	}
	return sum
}

// maxShapeDepth bounds recursion into nested struct fields. A component deeper than this is beyond
// what the fixture or any real component needs, and the bound removes any chance of a recursive
// type looping forever.
const maxShapeDepth = 8

// writeShape feeds one struct's shape into hash. Nested structs are followed, because a change
// inside one changes the bytes of every field that holds it.
func writeShape(hash io.Writer, t reflect.Type, depth int) {
	if depth > maxShapeDepth {
		return
	}

	for i := range t.NumField() {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue // unexported fields are not encoded
		}

		// The field's name and wire type are what a decoder has to agree on. Declaration order is
		// carried by the order these are written in, which is also the field number the generator
		// assigns.
		writeString(hash, f.Name)
		writeString(hash, ":")
		writeString(hash, wireKind(f.Type))
		if f.Type.Kind() == reflect.Struct {
			writeString(hash, "{")
			writeShape(hash, f.Type, depth+1)
			writeString(hash, "}")
		}
		writeString(hash, ";")
	}
}

func writeString(hash io.Writer, s string) {
	_, _ = io.WriteString(hash, s)
}

// wireKind returns the protobuf wire type a Go type encodes as.
//
// Wire types rather than Go types are what the hash records, so a change that leaves every byte
// identical does not trigger a migration. int32 to int64 is the usual example: both are varints,
// both encode the same values the same way, and nothing needs converting.
func wireKind(t reflect.Type) string {
	switch t.Kind() { //nolint:exhaustive // the default case names every other kind as opaque
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "varint"
	case reflect.Float32:
		return "fixed32"
	case reflect.Float64:
		return "fixed64"
	case reflect.String, reflect.Slice, reflect.Array, reflect.Map, reflect.Struct:
		return "bytes"
	default:
		// Pointers, interfaces and anything else a component may not hold. Named rather than
		// collapsed into bytes so an unexpected type cannot silently match a real one.
		return "opaque:" + t.Kind().String()
	}
}

// shapesMatch reports whether a stored shape may be decoded as the current one.
//
// unknownShape on either side means one of the two builds could not describe the component, which
// leaves nothing to compare. Those are treated as matching: claiming a mismatch from missing
// information would send a value through a migration on no evidence.
func shapesMatch(stored, current uint64) bool {
	if stored == unknownShape || current == unknownShape {
		return true
	}
	return stored == current
}

// The snapshot carries the shape table as field 5 of WorldState: one fixed64 per name table entry,
// in the same order, so a reader pairs the two by index. Protobuf emits fields in ascending tag
// order, so the two functions below run after the entities in both encoder passes.

// shapeTableWireSize is what the shape table adds to the encoded body, or zero when there is none.
func (ws *worldState) shapeTableWireSize() int {
	if !shapeTableNeeded(ws.components.shapes) {
		return 0
	}
	return protowire.SizeTag(shapeTableField) + protowire.SizeBytes(shapeTableSize(ws.components.shapes))
}

// appendShapeTableWire writes the shape table, or nothing when there is none. It must agree exactly
// with shapeTableWireSize: the encoder panics if the two passes disagree on length.
func (ws *worldState) appendShapeTableWire(buf []byte) []byte {
	shapes := ws.components.shapes
	if !shapeTableNeeded(shapes) {
		return buf
	}
	buf = protowire.AppendTag(buf, shapeTableField, protowire.BytesType)
	buf = protowire.AppendVarint(buf, uint64(shapeTableSize(shapes))) //nolint:gosec // non-negative
	for _, shape := range shapes {
		buf = protowire.AppendFixed64(buf, shape)
	}
	return buf
}

// shapeTableField is WorldState.component_hashes.
const shapeTableField protowire.Number = 5

// shapeTableSize is the packed size of the table: a flat run of eight-byte values.
func shapeTableSize(shapes []uint64) int {
	return len(shapes) * 8
}

// shapeTableNeeded reports whether a snapshot should carry a shape table at all. A world whose
// components are all undescribed would store a run of zeros saying nothing.
func shapeTableNeeded(shapes []uint64) bool {
	for _, shape := range shapes {
		if shape != unknownShape {
			return true
		}
	}
	return false
}
