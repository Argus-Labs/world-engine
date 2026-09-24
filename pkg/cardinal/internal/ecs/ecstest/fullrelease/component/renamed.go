package component

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/wire"

// Renamed is case 1c as the current build declares it: the field the early-access build called
// Before is now called After.
type Renamed struct {
	After uint32
}

func (Renamed) Name() string { return "renamed" }

func (c Renamed) SizeWire() int { return wire.SizeVarintField(1, c.After) }

func (c Renamed) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.After) }

func (c Renamed) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return Renamed{After: fields[1]}, nil
}

// RenamedV1 is the retired shape: renamed as the early-access build wrote it.
//
// On the V<n> suffix: real code is better off naming a retired shape after the release that wrote
// it, since the question an author has to answer is "can I delete this yet?" and that is about
// which releases are still in the wild. The fixture uses numbers because two of its retired shapes
// never shipped, so there is no release to name them after. It is declared here,
// beside the current shape, because it belongs to this build — it is this build's account of what
// the old bytes look like, written by hand from what the old code declared.
//
// Three things make it retired rather than current. It is never registered as a component, so no
// entity can hold one. Its Name matches the current shape, because a snapshot identifies a component
// by name and the stored bytes have to find their way here. And it only decodes: nothing in this
// build ever writes one.
//
// Writing it by hand is the riskiest thing in the feature. Get a field wrong and its shape hash no
// longer matches what the old build stored, so the migration that consumes it never fires and the
// restore refuses — loudly, which is the good failure. That is also why the fixture declares it in
// its own package rather than importing the early-access struct: sharing one Go type would make the
// two hashes equal by construction and the fixture could never catch the mistake.
type RenamedV1 struct {
	Before uint32
}

func (RenamedV1) Name() string { return "renamed" }

func (c RenamedV1) SizeWire() int { return wire.SizeVarintField(1, c.Before) }

func (c RenamedV1) AppendWire(b []byte) []byte { return wire.AppendVarintField(b, 1, c.Before) }

func (c RenamedV1) UnmarshalWire(data []byte) (any, error) {
	fields, err := wire.DecodeVarintFields(data)
	if err != nil {
		return nil, err
	}
	return RenamedV1{Before: fields[1]}, nil
}
