package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Absorber carries case 2c, merge many into an existing component. This one survives and gains
// [Absorbed]'s field.
type Absorber struct {
	A uint32
}

func (Absorber) Name() string { return "absorber" }

func (c Absorber) SizeWire() int { return wire.SizeVarintField(1, c.A) }

func (c Absorber) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.A) }

func (c Absorber) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Absorber{A: fields[1]}, nil
}
