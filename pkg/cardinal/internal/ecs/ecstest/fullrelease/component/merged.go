package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Merged is case 2b as the current build declares it: one new component holding what merge_left
// and merge_right each used to hold on their own.
//
// Neither old component is registered any more, so a save carrying them has two values that must
// arrive together at one migration. That is what makes 2b the first case needing an order: a
// migration with two inputs cannot run until both are available.
type Merged struct {
	A uint32
	B uint32
}

func (Merged) Name() string { return "merged" }

func (c Merged) SizeWire() int {
	return wire.SizeVarintField(1, c.A) + wire.SizeVarintField(2, c.B)
}

func (c Merged) AppendWire(b []byte) []byte {
	b = wire.AppendVarintField(b, 1, c.A)
	return wire.AppendVarintField(b, 2, c.B)
}

func (c Merged) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Merged{A: fields[1], B: fields[2]}, nil
}

// MergeLeftV1 is one of the two retired shapes 2b consumes.
type MergeLeftV1 struct {
	A uint32
}

func (MergeLeftV1) Name() string { return "merge_left" }

func (c MergeLeftV1) SizeWire() int { return wire.SizeVarintField(1, c.A) }

func (c MergeLeftV1) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.A) }

func (c MergeLeftV1) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return MergeLeftV1{A: fields[1]}, nil
}

// MergeRightV1 is the other retired shape 2b consumes.
type MergeRightV1 struct {
	B uint32
}

func (MergeRightV1) Name() string { return "merge_right" }

func (c MergeRightV1) SizeWire() int { return wire.SizeVarintField(1, c.B) }

func (c MergeRightV1) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.B) }

func (c MergeRightV1) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return MergeRightV1{B: fields[1]}, nil
}
