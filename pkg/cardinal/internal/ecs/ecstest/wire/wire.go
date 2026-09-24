// Package wire encodes the fixture's component values.
//
// Real components get SizeWire, AppendWire and UnmarshalWire from `world sdk generate`, which
// writes a wire.gen.go into each component package. The fixture writes them by hand for two
// reasons: the generator only emits for components wired to a system, and a fixture has no
// systems; and it emits nothing at all for retired shapes, which is exactly what a migration has
// to decode.
//
// The three helpers live in one package rather than being copied into each component package,
// since what the generator actually emits per package is the methods, not these.
//
// The encoding is proto3: a field holding the zero value is omitted, so a component whose fields
// are all zero encodes to no bytes.
package wire

import (
	"math"

	"google.golang.org/protobuf/encoding/protowire"
)

func SizeVarintField(num protowire.Number, value uint32) int {
	if value == 0 {
		return 0
	}
	return protowire.SizeTag(num) + protowire.SizeVarint(uint64(value))
}

func AppendVarintField(b []byte, num protowire.Number, value uint32) []byte {
	if value == 0 {
		return b
	}
	b = protowire.AppendTag(b, num, protowire.VarintType)
	return protowire.AppendVarint(b, uint64(value))
}

// DecodeVarintFields reads every varint field into a map keyed by field number. A field the encoder
// omitted is absent from the map and reads back as zero, which is what proto3 means by it.
func DecodeVarintFields(data []byte) (map[protowire.Number]uint32, error) {
	fields := map[protowire.Number]uint32{}
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		data = data[n:]
		if typ != protowire.VarintType {
			return nil, protowire.ParseError(-1)
		}
		v, n := protowire.ConsumeVarint(data)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		data = data[n:]
		fields[num] = uint32(v) //nolint:gosec // fixture components hold uint32 fields only
	}
	return fields, nil
}

// SizeFixed32Field, AppendFixed32Field and DecodeFixed32Fields are the fixed32 equivalents of the
// three above. They exist for the retype case: a field changing from an integer to a float changes
// its wire type, which is the only kind of retype a shape hash can see. An integer widening from
// 32 to 64 bits stays a varint and encodes identically, so nothing needs converting.

func SizeFixed32Field(num protowire.Number, value float32) int {
	if value == 0 {
		return 0
	}
	return protowire.SizeTag(num) + protowire.SizeFixed32()
}

func AppendFixed32Field(b []byte, num protowire.Number, value float32) []byte {
	if value == 0 {
		return b
	}
	b = protowire.AppendTag(b, num, protowire.Fixed32Type)
	return protowire.AppendFixed32(b, math.Float32bits(value))
}

func DecodeFixed32Fields(data []byte) (map[protowire.Number]float32, error) {
	fields := map[protowire.Number]float32{}
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		data = data[n:]
		if typ != protowire.Fixed32Type {
			return nil, protowire.ParseError(-1)
		}
		v, n := protowire.ConsumeFixed32(data)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		data = data[n:]
		fields[num] = math.Float32frombits(v)
	}
	return fields, nil
}
