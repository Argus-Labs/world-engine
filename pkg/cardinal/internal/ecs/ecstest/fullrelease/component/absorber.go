package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

const absorberName = "absorber"

// Absorber is case 2c as the current build declares it: a component that already existed, now
// holding the field absorbed used to carry.
//
// It differs from 2b in what happens to an entity that has only one of the two. Here the surviving
// component is still the entity's own, so the migration runs with the other input simply absent —
// which is what Optional is for.
type Absorber struct {
	A uint32
	B uint32
}

func (Absorber) Name() string { return absorberName }

func (c Absorber) SizeWire() int {
	return wire.SizeVarintField(1, c.A) + wire.SizeVarintField(2, c.B)
}

func (c Absorber) AppendWire(b []byte) []byte {
	b = wire.AppendVarintField(b, 1, c.A)
	return wire.AppendVarintField(b, 2, c.B)
}

func (c Absorber) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Absorber{A: fields[1], B: fields[2]}, nil
}

// AbsorberV1 is the retired shape of the surviving component, before it gained B.
type AbsorberV1 struct {
	A uint32
}

func (AbsorberV1) Name() string { return absorberName }

func (c AbsorberV1) SizeWire() int { return wire.SizeVarintField(1, c.A) }

func (c AbsorberV1) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.A) }

func (c AbsorberV1) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return AbsorberV1{A: fields[1]}, nil
}

// AbsorbedV1 is the retired shape of the component that goes away. This build does not register a
// current absorbed at all, so an entity that had one reaches the migration through this shape.
type AbsorbedV1 struct {
	B uint32
}

func (AbsorbedV1) Name() string { return "absorbed" }

func (c AbsorbedV1) SizeWire() int { return wire.SizeVarintField(1, c.B) }

func (c AbsorbedV1) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.B) }

func (c AbsorbedV1) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return AbsorbedV1{B: fields[1]}, nil
}
