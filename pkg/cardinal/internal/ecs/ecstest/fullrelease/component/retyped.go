package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Retyped is case 1e as the current build declares it: A is a float, so field 1 is fixed32 where
// the stored bytes hold a varint.
type Retyped struct {
	A float32
}

func (Retyped) Name() string { return "retyped" }

func (c Retyped) SizeWire() int { return wire.SizeFixed32Field(1, c.A) }

func (c Retyped) AppendWire(b []byte) []byte { return wire.AppendFixed32Field(b, 1, c.A) }

func (c Retyped) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeFixed32Fields(data)
	if err != nil {
		return nil, err
	}
	return Retyped{A: fields[1]}, nil
}

// RetypedV1 is the retired shape, where A was still an integer. It decodes a varint, which is the
// whole reason it has to exist: the current struct would reject these bytes outright.
type RetypedV1 struct {
	A uint32
}

func (RetypedV1) Name() string { return "retyped" }

func (c RetypedV1) SizeWire() int { return wire.SizeVarintField(1, c.A) }

func (c RetypedV1) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.A) }

func (c RetypedV1) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return RetypedV1{A: fields[1]}, nil
}
