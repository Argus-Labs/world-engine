// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file include/box2d/id.h.

package box2d

// WorldID references a world instance (upstream b2WorldId).
type WorldID struct {
	index1     uint16
	generation uint16
}

// BodyID references a body instance (upstream b2BodyId).
type BodyID struct {
	index1     int32
	world0     uint16
	generation uint16
}

// ShapeID references a shape instance (upstream b2ShapeId).
type ShapeID struct {
	index1     int32
	world0     uint16
	generation uint16
}

// ChainID references a chain instance (upstream b2ChainId).
type ChainID struct {
	index1     int32
	world0     uint16
	generation uint16
}

// JointID references a joint instance (upstream b2JointId).
type JointID struct {
	index1     int32
	world0     uint16
	generation uint16
}

// ContactID references a contact instance (upstream b2ContactId).
type ContactID struct {
	index1     int32
	world0     uint16
	padding    int16
	generation uint32
}

// IsNull reports whether the id is null (upstream B2_IS_NULL).
func (id ContactID) IsNull() bool { return id.index1 == 0 }

// IsNonNull reports whether the id is non-null (upstream B2_IS_NON_NULL).
func (id ContactID) IsNonNull() bool { return id.index1 != 0 }

// PackContactID stores a contact id into three uint32s (upstream b2StoreContactId).
//
// Slot 0 is the contact's 1-based dense per-world index (index1: the contact
// pool index plus one, with 0 reserved for null) — a documented part of the
// format, not an accident of layout: callers may use it as a compact table
// key for live contacts (physics2d's contact gather does).
// TestOracleContactID_CReference pins the layout.
func PackContactID(id ContactID) [3]uint32 {
	return [3]uint32{uint32(id.index1), uint32(id.world0), id.generation}
}

// UnpackContactID loads three uint32s into a contact id (upstream b2LoadContactId).
func UnpackContactID(values [3]uint32) ContactID {
	return ContactID{
		index1:     int32(values[0]),
		world0:     uint16(values[1]),
		padding:    0,
		generation: values[2],
	}
}

// IsNull reports whether the id is null (upstream B2_IS_NULL).
func (id WorldID) IsNull() bool { return id.index1 == 0 }

// IsNonNull reports whether the id is non-null (upstream B2_IS_NON_NULL).
func (id WorldID) IsNonNull() bool { return id.index1 != 0 }

// IsNull reports whether the id is null (upstream B2_IS_NULL).
func (id BodyID) IsNull() bool { return id.index1 == 0 }

// IsNonNull reports whether the id is non-null (upstream B2_IS_NON_NULL).
func (id BodyID) IsNonNull() bool { return id.index1 != 0 }

// IsNull reports whether the id is null (upstream B2_IS_NULL).
func (id ShapeID) IsNull() bool { return id.index1 == 0 }

// IsNonNull reports whether the id is non-null (upstream B2_IS_NON_NULL).
func (id ShapeID) IsNonNull() bool { return id.index1 != 0 }

// IsNull reports whether the id is null (upstream B2_IS_NULL).
func (id ChainID) IsNull() bool { return id.index1 == 0 }

// IsNonNull reports whether the id is non-null (upstream B2_IS_NON_NULL).
func (id ChainID) IsNonNull() bool { return id.index1 != 0 }

// IsNull reports whether the id is null (upstream B2_IS_NULL).
func (id JointID) IsNull() bool { return id.index1 == 0 }

// IsNonNull reports whether the id is non-null (upstream B2_IS_NON_NULL).
func (id JointID) IsNonNull() bool { return id.index1 != 0 }

// PackWorldID stores a world id into a uint32 (upstream b2StoreWorldId).
func PackWorldID(id WorldID) uint32 {
	return (uint32(id.index1) << 16) | uint32(id.generation)
}

// UnpackWorldID loads a uint32 into a world id (upstream b2LoadWorldId).
func UnpackWorldID(x uint32) WorldID {
	return WorldID{
		index1:     uint16(x >> 16),
		generation: uint16(x),
	}
}

// PackBodyID stores a body id into a uint64 (upstream b2StoreBodyId).
func PackBodyID(id BodyID) uint64 {
	return (uint64(uint32(id.index1)) << 32) | (uint64(id.world0) << 16) | uint64(id.generation)
}

// UnpackBodyID loads a uint64 into a body id (upstream b2LoadBodyId).
func UnpackBodyID(x uint64) BodyID {
	return BodyID{
		index1:     int32(x >> 32),
		world0:     uint16(x >> 16),
		generation: uint16(x),
	}
}

// PackShapeID stores a shape id into a uint64 (upstream b2StoreShapeId).
func PackShapeID(id ShapeID) uint64 {
	return (uint64(uint32(id.index1)) << 32) | (uint64(id.world0) << 16) | uint64(id.generation)
}

// UnpackShapeID loads a uint64 into a shape id (upstream b2LoadShapeId).
func UnpackShapeID(x uint64) ShapeID {
	return ShapeID{
		index1:     int32(x >> 32),
		world0:     uint16(x >> 16),
		generation: uint16(x),
	}
}

// PackChainID stores a chain id into a uint64 (upstream b2StoreChainId).
func PackChainID(id ChainID) uint64 {
	return (uint64(uint32(id.index1)) << 32) | (uint64(id.world0) << 16) | uint64(id.generation)
}

// UnpackChainID loads a uint64 into a chain id (upstream b2LoadChainId).
func UnpackChainID(x uint64) ChainID {
	return ChainID{
		index1:     int32(x >> 32),
		world0:     uint16(x >> 16),
		generation: uint16(x),
	}
}

// PackJointID stores a joint id into a uint64 (upstream b2StoreJointId).
func PackJointID(id JointID) uint64 {
	return (uint64(uint32(id.index1)) << 32) | (uint64(id.world0) << 16) | uint64(id.generation)
}

// UnpackJointID loads a uint64 into a joint id (upstream b2LoadJointId).
func UnpackJointID(x uint64) JointID {
	return JointID{
		index1:     int32(x >> 32),
		world0:     uint16(x >> 16),
		generation: uint16(x),
	}
}
