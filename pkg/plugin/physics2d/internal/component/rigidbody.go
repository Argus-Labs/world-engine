package component

// BodyType selects how the rigid body participates in the simulation.
type BodyType uint8

const (
	// BodyTypeStatic is immovable world geometry; zero velocity; does not respond to forces.
	BodyTypeStatic BodyType = iota + 1
	// BodyTypeDynamic is fully simulated: forces, collisions, and integration apply.
	BodyTypeDynamic
	// BodyTypeKinematic is moved by setting velocity from gameplay; Box2D integrates velocity
	// into position each step. Does not respond to forces but can push dynamic bodies on
	// contact. Post-step writeback keeps ECS in sync with Box2D's integrated position.
	BodyTypeKinematic
	// BodyTypeManual is for gameplay-driven entities that use Box2D only for contact detection.
	// Under the hood it creates a kinematic body, but post-step writeback is skipped: ECS owns
	// position and velocity, and the reconciler pushes ECS values into Box2D each tick.
	// Use this for characters, enemies, and other entities where gameplay code (input handling,
	// AI, pathfinding) computes position directly.
	//
	// Box2D collision rules apply: manual bodies generate contacts with dynamic bodies only,
	// not with static or other kinematic/manual bodies.
	BodyTypeManual
)
