package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// MovedTo carries half of case 2d, move data between surviving components. It gains a field that
// [MovedFrom] gives up. Both components survive, which is what separates 2d from a merge.
type MovedTo struct {
	C uint32
}

func (MovedTo) Name() string { return "moved_to" }

func (c MovedTo) SizeWire() int { return wire.SizeVarintField(1, c.C) }

func (c MovedTo) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.C) }

func (c MovedTo) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return MovedTo{C: fields[1]}, nil
}
