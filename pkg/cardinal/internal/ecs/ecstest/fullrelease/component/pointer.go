package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

const pointerName = "pointer"

// Pointer is case 3c as the current build declares it: it keeps the slot it always named, and
// gains the entity ID that slot resolves to.
//
// The slot has to survive the per-entity migration, because resolving it needs every slotted
// entity and only the whole-world pass can see those. Target is zero until then.
type Pointer struct {
	Slot   uint32
	Target uint32
}

func (Pointer) Name() string { return pointerName }

func (c Pointer) SizeWire() int {
	return wire.SizeVarintField(1, c.Slot) + wire.SizeVarintField(2, c.Target)
}

func (c Pointer) AppendWire(b []byte) []byte {
	b = wire.AppendVarintField(b, 1, c.Slot)
	return wire.AppendVarintField(b, 2, c.Target)
}

func (c Pointer) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Pointer{Slot: fields[1], Target: fields[2]}, nil
}

// PointerV1 is the retired shape, which named its target by slot alone.
type PointerV1 struct {
	Slot uint32
}

func (PointerV1) Name() string { return pointerName }

func (c PointerV1) SizeWire() int { return wire.SizeVarintField(1, c.Slot) }

func (c PointerV1) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.Slot) }

func (c PointerV1) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return PointerV1{Slot: fields[1]}, nil
}
