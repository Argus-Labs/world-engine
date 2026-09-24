package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Retyped carries case 1e, retype a field. Here A is an integer; the full-release build makes it a
// float.
//
// Only a retype that changes the wire type is detectable. Widening uint32 to uint64 leaves every
// byte and every hash identical, so no migration runs and none is needed. Integer to float changes
// varint to fixed32, which the hash sees.
type Retyped struct {
	A uint32
}

func (Retyped) Name() string { return "retyped" }

func (c Retyped) SizeWire() int { return wire.SizeVarintField(1, c.A) }

func (c Retyped) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.A) }

func (c Retyped) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Retyped{A: fields[1]}, nil
}
