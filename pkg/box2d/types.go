// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file include/box2d/types.h and src/types.c.

package box2d

import "math"

// DefaultCategoryBits is B2_DEFAULT_CATEGORY_BITS.
const DefaultCategoryBits uint64 = 1

// DefaultMaskBits is B2_DEFAULT_MASK_BITS (UINT64_MAX).
const DefaultMaskBits uint64 = math.MaxUint64

// NOTE: the upstream task interface (b2TaskCallback, b2EnqueueTaskCallback,
// b2FinishTaskCallback) is not ported. This port executes single-threaded, so
// the task callbacks and b2WorldDef.workerCount have no meaning here.

// FrictionCallback is an optional friction mixing callback. The default uses
// sqrt(frictionA * frictionB). This intentionally provides no context object
// because upstream calls it from a worker thread.
//
// Warning: this function should not attempt to modify Box2D state or user
// application state.
type FrictionCallback func(frictionA float64, userMaterialIDA uint64, frictionB float64, userMaterialIDB uint64) float64

// RestitutionCallback is an optional restitution mixing callback. The default
// uses max(restitutionA, restitutionB). This intentionally provides no context
// object because upstream calls it from a worker thread.
//
// Warning: this function should not attempt to modify Box2D state or user
// application state.
type RestitutionCallback func(restitutionA float64, userMaterialIDA uint64, restitutionB float64, userMaterialIDB uint64) float64

// RayResult is the result from World.RayCastClosest (upstream b2RayResult).
// If there is initial overlap the fraction and normal will be zero while the
// point is an arbitrary point in the overlap region.
type RayResult struct {
	ShapeID    ShapeID
	Point      Vec2
	Normal     Vec2
	Fraction   float64
	NodeVisits int
	LeafVisits int
	Hit        bool
}

// WorldDef is a world definition used to create a simulation world
// (upstream b2WorldDef). Must be initialized using DefaultWorldDef.
//
// Deviations from upstream: the task-system fields (workerCount, enqueueTask,
// finishTask, userTaskContext) are not ported because this port executes
// single-threaded. The b2_secretCookie/internalValue guard is replaced by the
// private initialized flag set by DefaultWorldDef.
type WorldDef struct {
	// Gravity vector. Box2D has no up-vector defined.
	Gravity Vec2

	// Restitution speed threshold, usually in m/s. Collisions above this
	// speed have restitution applied (will bounce).
	RestitutionThreshold float64

	// Threshold speed for hit events. Usually meters per second.
	HitEventThreshold float64

	// Contact stiffness. Cycles per second. Increasing this increases the speed of overlap recovery, but can introduce jitter.
	ContactHertz float64

	// Contact bounciness. Non-dimensional. You can speed up overlap recovery by decreasing this with
	// the trade-off that overlap resolution becomes more energetic.
	ContactDampingRatio float64

	// This parameter controls how fast overlap is resolved and usually has units of meters per second. This only
	// puts a cap on the resolution speed. The resolution speed is increased by increasing the hertz and/or
	// decreasing the damping ratio.
	ContactSpeed float64

	// Maximum linear speed. Usually meters per second.
	MaximumLinearSpeed float64

	// Optional mixing callback for friction. The default uses sqrt(frictionA * frictionB).
	FrictionCallback FrictionCallback

	// Optional mixing callback for restitution. The default uses max(restitutionA, restitutionB).
	RestitutionCallback RestitutionCallback

	// Can bodies go to sleep to improve performance
	EnableSleep bool

	// Enable continuous collision
	EnableContinuous bool

	// Contact softening when mass ratios are large. Experimental.
	EnableContactSoftening bool

	// User data. Deviation from upstream: the C void* becomes a uint64 so the
	// ECS wrapper can pack an entity id directly.
	UserData uint64

	// initialized replaces the upstream internalValue/B2_SECRET_COOKIE guard.
	// It is set by DefaultWorldDef. Do not set it yourself: NewWorld always
	// checks it and panics on a definition that skipped the constructor.
	initialized bool
}

// BodyType is the body simulation type (upstream b2BodyType).
// Each body is one of these three types. The type determines how the body
// behaves in the simulation.
type BodyType int32

const (
	// StaticBody has zero mass, zero velocity, and may be manually moved.
	StaticBody BodyType = 0

	// KinematicBody has zero mass, velocity set by user, and is moved by the solver.
	KinematicBody BodyType = 1

	// DynamicBody has positive mass, velocity determined by forces, and is moved by the solver.
	DynamicBody BodyType = 2

	// BodyTypeCount is the number of body types.
	BodyTypeCount BodyType = 3
)

// MotionLocks holds motion locks to restrict the body movement
// (upstream b2MotionLocks).
type MotionLocks struct {
	// Prevent translation along the x-axis
	LinearX bool

	// Prevent translation along the y-axis
	LinearY bool

	// Prevent rotation around the z-axis
	AngularZ bool
}

// BodyDef holds all the data needed to construct a rigid body
// (upstream b2BodyDef).
// You can safely re-use body definitions. Shapes are added to a body after
// construction. Body definitions are temporary objects used to bundle creation
// parameters. Must be initialized using DefaultBodyDef.
type BodyDef struct {
	// The body type: static, kinematic, or dynamic.
	Type BodyType

	// The initial world position of the body. Bodies should be created with the desired position.
	//
	// Note: creating bodies at the origin and then moving them nearly doubles the cost of body creation, especially
	// if the body is moved after shapes have been added.
	Position Vec2

	// The initial world rotation of the body. Use MakeRot if you have an angle.
	Rotation Rot

	// The initial linear velocity of the body's origin. Usually in meters per second.
	LinearVelocity Vec2

	// The initial angular velocity of the body. Radians per second.
	AngularVelocity float64

	// Linear damping is used to reduce the linear velocity. The damping parameter
	// can be larger than 1 but the damping effect becomes sensitive to the
	// time step when the damping parameter is large.
	// Generally linear damping is undesirable because it makes objects move slowly
	// as if they are floating.
	LinearDamping float64

	// Angular damping is used to reduce the angular velocity. The damping parameter
	// can be larger than 1.0 but the damping effect becomes sensitive to the
	// time step when the damping parameter is large.
	// Angular damping can be use slow down rotating bodies.
	AngularDamping float64

	// Scale the gravity applied to this body. Non-dimensional.
	GravityScale float64

	// Sleep speed threshold, default is 0.05 meters per second
	SleepThreshold float64

	// Optional body name for debugging. Up to 31 characters.
	Name string

	// Use this to store application specific body data. Deviation from
	// upstream: the C void* becomes a uint64 so the ECS wrapper can pack an
	// entity id directly.
	UserData uint64

	// Motions locks to restrict linear and angular movement.
	// Caution: may lead to softer constraints along the locked direction
	MotionLocks MotionLocks

	// Set this flag to false if this body should never fall asleep.
	EnableSleep bool

	// Is this body initially awake or sleeping?
	IsAwake bool

	// Treat this body as a high speed object that performs continuous collision detection
	// against dynamic and kinematic bodies, but not other bullet bodies.
	//
	// Warning: bullets should be used sparingly. They are not a solution for general dynamic-versus-dynamic
	// continuous collision. They do not guarantee accurate collision if both bodies are fast moving because
	// the bullet does a continuous check after all non-bullet bodies have moved. You could get unlucky and have
	// the bullet body end a time step very close to a non-bullet body and the non-bullet body then moves over
	// the bullet body. In continuous collision, initial overlap is ignored to avoid freezing bodies in place.
	// I do not recommend using them for game projectiles if precise collision timing is needed. Instead consider
	// using a ray or shape cast. You can use a marching ray or shape cast for projectile that moves over time.
	// If you want a fast moving projectile to collide with a fast moving target, you need to consider the relative
	// movement in your ray or shape cast. This is out of the scope of Box2D.
	// So what are good use cases for bullets? Pinball games or games with dynamic containers that hold other objects.
	// It should be a use case where it doesn't break the game if there is a collision missed, but the having them
	// captured improves the quality of the game.
	IsBullet bool

	// Used to disable a body. A disabled body does not move or collide.
	IsEnabled bool

	// This allows this body to bypass rotational speed limits. Should only be used
	// for circular objects, like wheels.
	AllowFastRotation bool

	// initialized replaces the upstream internalValue/B2_SECRET_COOKIE guard.
	// It is set by DefaultBodyDef. Do not set it yourself: World.CreateBody
	// always checks it and panics on a definition that skipped the constructor.
	initialized bool
}

// Filter is used to filter collision on shapes (upstream b2Filter). It affects
// shape-vs-shape collision and shape-versus-query collision (such as
// World.CastRay).
type Filter struct {
	// The collision category bits. Normally you would just set one bit. The category bits should
	// represent your application object types. For example:
	//
	//	const (
	//		CategoryStatic  = 0x00000001
	//		CategoryDynamic = 0x00000002
	//		CategoryDebris  = 0x00000004
	//		CategoryPlayer  = 0x00000008
	//		// etc
	//	)
	CategoryBits uint64

	// The collision mask bits. This states the categories that this
	// shape would accept for collision.
	// For example, you may want your player to only collide with static objects
	// and other players:
	//
	//	maskBits = CategoryStatic | CategoryPlayer
	MaskBits uint64

	// Collision groups allow a certain group of objects to never collide (negative)
	// or always collide (positive). A group index of zero has no effect. Non-zero group filtering
	// always wins against the mask bits.
	// For example, you may want ragdolls to collide with other ragdolls but you don't want
	// ragdoll self-collision. In this case you would give each ragdoll a unique negative group index
	// and apply that group index to all shapes on the ragdoll.
	GroupIndex int
}

// QueryFilter is used to filter collisions between queries and shapes
// (upstream b2QueryFilter). For example, you may want a ray-cast representing
// a projectile to hit players and the static environment but not debris.
type QueryFilter struct {
	// The collision category bits of this query. Normally you would just set one bit.
	CategoryBits uint64

	// The collision mask bits. This states the shape categories that this
	// query would accept for collision.
	MaskBits uint64
}

// ShapeType is the shape type (upstream b2ShapeType).
type ShapeType int32

const (
	// CircleShape is a circle with an offset.
	CircleShape ShapeType = iota

	// CapsuleShape is an extruded circle.
	CapsuleShape

	// SegmentShape is a line segment.
	SegmentShape

	// PolygonShape is a convex polygon.
	PolygonShape

	// ChainSegmentShape is a line segment owned by a chain shape.
	ChainSegmentShape

	// ShapeTypeCount is the number of shape types.
	ShapeTypeCount
)

// SurfaceMaterial allows chain shapes to have per segment surface properties
// (upstream b2SurfaceMaterial).
type SurfaceMaterial struct {
	// The Coulomb (dry) friction coefficient, usually in the range [0,1].
	Friction float64

	// The coefficient of restitution (bounce) usually in the range [0,1].
	// https://en.wikipedia.org/wiki/Coefficient_of_restitution
	Restitution float64

	// The rolling resistance usually in the range [0,1].
	RollingResistance float64

	// The tangent speed for conveyor belts
	TangentSpeed float64

	// User material identifier. This is passed with query results and to friction and restitution
	// combining functions. It is not used internally.
	UserMaterialID uint64

	// Custom debug draw color.
	CustomColor uint32
}

// ShapeDef is used to create a shape (upstream b2ShapeDef).
// This is a temporary object used to bundle shape creation parameters. You may
// use the same shape definition to create multiple shapes.
// Must be initialized using DefaultShapeDef.
type ShapeDef struct {
	// Use this to store application specific shape data. Deviation from
	// upstream: the C void* becomes a uint64 so the ECS wrapper can pack an
	// entity id directly.
	UserData uint64

	// The surface material for this shape.
	Material SurfaceMaterial

	// The density, usually in kg/m^2.
	// This is not part of the surface material because this is for the interior, which may have
	// other considerations, such as being hollow. For example a wood barrel may be hollow or full of water.
	Density float64

	// Collision filtering data.
	Filter Filter

	// Enable custom filtering. Only one of the two shapes needs to enable custom filtering. See WorldDef.
	EnableCustomFiltering bool

	// A sensor shape generates overlap events but never generates a collision response.
	// Sensors do not have continuous collision. Instead, use a ray or shape cast for those scenarios.
	// Sensors still contribute to the body mass if they have non-zero density.
	//
	// Note: sensor events are disabled by default. See EnableSensorEvents.
	IsSensor bool

	// Enable sensor events for this shape. This applies to sensors and non-sensors. Both shapes involved must have this flag set to true.
	// False by default, even for sensors.
	EnableSensorEvents bool

	// Enable contact events for this shape. Only applies to kinematic and dynamic bodies. Only one shape involved needs this flag set to true.
	// Ignored for sensors. False by default.
	EnableContactEvents bool

	// Enable hit events for this shape. Only applies to kinematic and dynamic bodies. Only one shape involved needs this flag set to true.
	// Ignored for sensors. False by default.
	EnableHitEvents bool

	// Enable pre-solve contact events for this shape. Only applies to dynamic bodies. These are expensive
	// and must be carefully handled due to multithreading. Ignored for sensors.
	EnablePreSolveEvents bool

	// When shapes are created they will scan the environment for collision the next time step. This can significantly slow down
	// static body creation when there are many static shapes.
	// This is flag is ignored for dynamic and kinematic shapes which always invoke contact creation.
	InvokeContactCreation bool

	// Should the body update the mass properties when this shape is created. Default is true.
	// Set this to false to skip the recomputation while adding many shapes to one body; the
	// body is then left with stale mass data, so call World.ApplyBodyMassFromShapes for it
	// before simulating.
	UpdateBodyMass bool

	// initialized replaces the upstream internalValue/B2_SECRET_COOKIE guard.
	// It is set by DefaultShapeDef. Do not set it yourself: the World.Create*Shape
	// functions always check it and panic on a definition that skipped the
	// constructor.
	initialized bool
}

// ChainDef is used to create a chain of line segments (upstream b2ChainDef).
// This is designed to eliminate ghost collisions with some limitations.
//   - chains are one-sided
//   - chains have no mass and should be used on static bodies
//   - chains have a counter-clockwise winding order (normal points right of segment direction)
//   - chains are either a loop or open
//   - a chain must have at least 4 points
//   - the distance between any two points must be greater than LinearSlop
//   - a chain shape should not self intersect (this is not validated)
//   - an open chain shape has NO COLLISION on the first and final edge
//   - you may overlap two open chains on their first three and/or last three points to get smooth collision
//   - a chain shape creates multiple line segment shapes on the body
//
// https://en.wikipedia.org/wiki/Polygonal_chain
// Must be initialized using DefaultChainDef.
//
// Warning: do not use chain shapes unless you understand the limitations. This
// is an advanced feature.
//
// Deviation from upstream: the pointer+count pairs (points/count and
// materials/materialCount) become slices; use len to recover the counts.
type ChainDef struct {
	// Use this to store application specific shape data. Deviation from
	// upstream: the C void* becomes a uint64 so the ECS wrapper can pack an
	// entity id directly.
	UserData uint64

	// At least 4 points. These are cloned and may be temporary.
	Points []Vec2

	// Surface materials for each segment. These are cloned.
	// The length must be 1 or len(Points). This allows you to provide one
	// material for all segments or a unique material per segment. For open
	// chains, the material on the ghost segments are place holders.
	Materials []SurfaceMaterial

	// Contact filtering data.
	Filter Filter

	// Indicates a closed chain formed by connecting the first and last points
	IsLoop bool

	// Enable sensors to detect this chain. False by default.
	EnableSensorEvents bool

	// initialized replaces the upstream internalValue/B2_SECRET_COOKIE guard.
	// It is set by DefaultChainDef. Do not set it yourself: World.CreateChain
	// always checks it and panics on a definition that skipped the constructor.
	initialized bool
}

// Profile holds profiling data. Times are in milliseconds (upstream b2Profile).
type Profile struct {
	Step                float64
	Pairs               float64
	Collide             float64
	Solve               float64
	PrepareStages       float64
	SolveConstraints    float64
	PrepareConstraints  float64
	IntegrateVelocities float64
	WarmStart           float64
	SolveImpulses       float64
	IntegratePositions  float64
	RelaxImpulses       float64
	ApplyRestitution    float64
	StoreImpulses       float64
	SplitIslands        float64
	Transforms          float64
	SensorHits          float64
	JointEvents         float64
	HitEvents           float64
	Refit               float64
	Bullets             float64
	SleepIslands        float64
	Sensors             float64
}

// Counters give details of the simulation size (upstream b2Counters).
type Counters struct {
	BodyCount        int
	ShapeCount       int
	ContactCount     int
	JointCount       int
	IslandCount      int
	StackUsed        int
	StaticTreeHeight int
	TreeHeight       int
	ByteCount        int
	TaskCount        int
	ColorCounts      [GraphColorCount]int
}

// JointType is the joint type enumeration (upstream b2JointType).
//
// This is useful because all joint types use JointID and sometimes you want to
// get the type of a joint.
type JointType int32

const (
	DistanceJoint JointType = iota
	FilterJoint
	MotorJoint
	PrismaticJoint
	RevoluteJoint
	WeldJoint
	WheelJoint
)

// JointDef is the base joint definition used by all joint types
// (upstream b2JointDef).
// The local frames are measured from the body's origin rather than the center
// of mass because:
//  1. you might not know where the center of mass will be
//  2. if you add/remove shapes from a body and recompute the mass, the joints will be broken
type JointDef struct {
	// User data. Deviation from upstream: the C void* becomes a uint64 so the
	// ECS wrapper can pack an entity id directly.
	UserData uint64

	// The first attached body
	BodyIDA BodyID

	// The second attached body
	BodyIDB BodyID

	// The first local joint frame
	LocalFrameA Transform

	// The second local joint frame
	LocalFrameB Transform

	// Force threshold for joint events
	ForceThreshold float64

	// Torque threshold for joint events
	TorqueThreshold float64

	// Constraint hertz (advanced feature)
	ConstraintHertz float64

	// Constraint damping ratio (advanced feature)
	ConstraintDampingRatio float64

	// Debug draw scale
	DrawScale float64

	// Set this flag to true if the attached bodies should collide
	CollideConnected bool
}

// DistanceJointDef is a distance joint definition
// (upstream b2DistanceJointDef).
// Connects a point on body A with a point on body B by a segment.
// Useful for ropes and springs.
type DistanceJointDef struct {
	// Base joint definition
	Base JointDef

	// The rest length of this joint. Clamped to a stable minimum value.
	Length float64

	// Enable the distance constraint to behave like a spring. If false
	// then the distance joint will be rigid, overriding the limit and motor.
	EnableSpring bool

	// The lower spring force controls how much tension it can sustain
	LowerSpringForce float64

	// The upper spring force controls how much compression it an sustain
	UpperSpringForce float64

	// The spring linear stiffness Hertz, cycles per second
	Hertz float64

	// The spring linear damping ratio, non-dimensional
	DampingRatio float64

	// Enable/disable the joint limit
	EnableLimit bool

	// Minimum length for limit. Clamped to a stable minimum value.
	MinLength float64

	// Maximum length for limit. Must be greater than or equal to the minimum length.
	MaxLength float64

	// Enable/disable the joint motor
	EnableMotor bool

	// The maximum motor force, usually in newtons
	MaxMotorForce float64

	// The desired motor speed, usually in meters per second
	MotorSpeed float64

	// initialized replaces the upstream internalValue/B2_SECRET_COOKIE guard.
	// It is set by DefaultDistanceJointDef (ported with src/joint.c). Do not set it
	// yourself: World.CreateDistanceJoint always checks it and panics on a
	// definition that skipped the constructor.
	initialized bool
}

// MotorJointDef is used to control the relative velocity and or transform
// between two bodies (upstream b2MotorJointDef). With a velocity of zero this
// acts like top-down friction.
type MotorJointDef struct {
	// Base joint definition
	Base JointDef

	// The desired linear velocity
	LinearVelocity Vec2

	// The maximum motor force in newtons
	MaxVelocityForce float64

	// The desired angular velocity
	AngularVelocity float64

	// The maximum motor torque in newton-meters
	MaxVelocityTorque float64

	// Linear spring hertz for position control
	LinearHertz float64

	// Linear spring damping ratio
	LinearDampingRatio float64

	// Maximum spring force in newtons
	MaxSpringForce float64

	// Angular spring hertz for position control
	AngularHertz float64

	// Angular spring damping ratio
	AngularDampingRatio float64

	// Maximum spring torque in newton-meters
	MaxSpringTorque float64

	// initialized replaces the upstream internalValue/B2_SECRET_COOKIE guard.
	// It is set by DefaultMotorJointDef (ported with src/joint.c). Do not set it
	// yourself: World.CreateMotorJoint always checks it and panics on a
	// definition that skipped the constructor.
	initialized bool
}

// FilterJointDef is used to disable collision between two specific bodies
// (upstream b2FilterJointDef).
type FilterJointDef struct {
	// Base joint definition
	Base JointDef

	// initialized replaces the upstream internalValue/B2_SECRET_COOKIE guard.
	// It is set by DefaultFilterJointDef (ported with src/joint.c). Do not set it
	// yourself: World.CreateFilterJoint always checks it and panics on a
	// definition that skipped the constructor.
	initialized bool
}

// PrismaticJointDef is a prismatic joint definition
// (upstream b2PrismaticJointDef).
// Body B may slide along the x-axis in local frame A. Body B cannot rotate
// relative to body A. The joint translation is zero when the local frame
// origins coincide in world space.
type PrismaticJointDef struct {
	// Base joint definition
	Base JointDef

	// Enable a linear spring along the prismatic joint axis
	EnableSpring bool

	// The spring stiffness Hertz, cycles per second
	Hertz float64

	// The spring damping ratio, non-dimensional
	DampingRatio float64

	// The target translation for the joint in meters. The spring-damper will drive
	// to this translation.
	TargetTranslation float64

	// Enable/disable the joint limit
	EnableLimit bool

	// The lower translation limit
	LowerTranslation float64

	// The upper translation limit
	UpperTranslation float64

	// Enable/disable the joint motor
	EnableMotor bool

	// The maximum motor force, typically in newtons
	MaxMotorForce float64

	// The desired motor speed, typically in meters per second
	MotorSpeed float64

	// initialized replaces the upstream internalValue/B2_SECRET_COOKIE guard.
	// It is set by DefaultPrismaticJointDef (ported with src/joint.c). Do not set it
	// yourself: World.CreatePrismaticJoint always checks it and panics on a
	// definition that skipped the constructor.
	initialized bool
}

// RevoluteJointDef is a revolute joint definition
// (upstream b2RevoluteJointDef).
// A point on body B is fixed to a point on body A. Allows relative rotation.
type RevoluteJointDef struct {
	// Base joint definition
	Base JointDef

	// The target angle for the joint in radians. The spring-damper will drive
	// to this angle.
	TargetAngle float64

	// Enable a rotational spring on the revolute hinge axis
	EnableSpring bool

	// The spring stiffness Hertz, cycles per second
	Hertz float64

	// The spring damping ratio, non-dimensional
	DampingRatio float64

	// A flag to enable joint limits
	EnableLimit bool

	// The lower angle for the joint limit in radians. Minimum of -0.99*pi radians.
	LowerAngle float64

	// The upper angle for the joint limit in radians. Maximum of 0.99*pi radians.
	UpperAngle float64

	// A flag to enable the joint motor
	EnableMotor bool

	// The maximum motor torque, typically in newton-meters
	MaxMotorTorque float64

	// The desired motor speed in radians per second
	MotorSpeed float64

	// initialized replaces the upstream internalValue/B2_SECRET_COOKIE guard.
	// It is set by DefaultRevoluteJointDef (ported with src/joint.c). Do not set it
	// yourself: World.CreateRevoluteJoint always checks it and panics on a
	// definition that skipped the constructor.
	initialized bool
}

// WeldJointDef is a weld joint definition (upstream b2WeldJointDef).
// Connects two bodies together rigidly. This constraint provides springs to
// mimic soft-body simulation.
//
// Note: the approximate solver in Box2D cannot hold many bodies together rigidly.
type WeldJointDef struct {
	// Base joint definition
	Base JointDef

	// Linear stiffness expressed as Hertz (cycles per second). Use zero for maximum stiffness.
	LinearHertz float64

	// Angular stiffness as Hertz (cycles per second). Use zero for maximum stiffness.
	AngularHertz float64

	// Linear damping ratio, non-dimensional. Use 1 for critical damping.
	LinearDampingRatio float64

	// Linear damping ratio, non-dimensional. Use 1 for critical damping.
	AngularDampingRatio float64

	// initialized replaces the upstream internalValue/B2_SECRET_COOKIE guard.
	// It is set by DefaultWeldJointDef (ported with src/joint.c). Do not set it
	// yourself: World.CreateWeldJoint always checks it and panics on a
	// definition that skipped the constructor.
	initialized bool
}

// WheelJointDef is a wheel joint definition (upstream b2WheelJointDef).
// Body B is a wheel that may rotate freely and slide along the local x-axis in
// frame A. The joint translation is zero when the local frame origins coincide
// in world space.
type WheelJointDef struct {
	// Base joint definition
	Base JointDef

	// Enable a linear spring along the local axis
	EnableSpring bool

	// Spring stiffness in Hertz
	Hertz float64

	// Spring damping ratio, non-dimensional
	DampingRatio float64

	// Enable/disable the joint linear limit
	EnableLimit bool

	// The lower translation limit
	LowerTranslation float64

	// The upper translation limit
	UpperTranslation float64

	// Enable/disable the joint rotational motor
	EnableMotor bool

	// The maximum motor torque, typically in newton-meters
	MaxMotorTorque float64

	// The desired motor speed in radians per second
	MotorSpeed float64

	// initialized replaces the upstream internalValue/B2_SECRET_COOKIE guard.
	// It is set by DefaultWheelJointDef (ported with src/joint.c). Do not set it
	// yourself: World.CreateWheelJoint always checks it and panics on a
	// definition that skipped the constructor.
	initialized bool
}

// ExplosionDef is used to configure options for explosions
// (upstream b2ExplosionDef). Explosions consider shape geometry when computing
// the impulse.
type ExplosionDef struct {
	// Mask bits to filter shapes
	MaskBits uint64

	// The center of the explosion in world space
	Position Vec2

	// The radius of the explosion
	Radius float64

	// The falloff distance beyond the radius. Impulse is reduced to zero at this distance.
	Falloff float64

	// Impulse per unit length. This applies an impulse according to the shape perimeter that
	// is facing the explosion. Explosions only apply to circles, capsules, and polygons. This
	// may be negative for implosions.
	ImpulsePerLength float64
}

// World event types.
//
// Events are used to collect events that occur during the world time step.
// These events are then available to query after the time step is complete.
//
// Also when events occur in the simulation step it may be problematic to
// modify the world, which is often what applications want to do when events
// occur.
//
// With event slices, you can scan the events in a loop and modify the world.
// However, you need to be careful that some event data may become invalid.

// SensorBeginTouchEvent is generated when a shape starts to overlap a sensor
// shape (upstream b2SensorBeginTouchEvent).
type SensorBeginTouchEvent struct {
	// The id of the sensor shape
	SensorShapeID ShapeID

	// The id of the shape that began touching the sensor shape
	VisitorShapeID ShapeID
}

// SensorEndTouchEvent is generated when a shape stops overlapping a sensor
// shape (upstream b2SensorEndTouchEvent).
// These include things like setting the transform, destroying a body or shape,
// or changing a filter. You will also get an end event if the sensor or
// visitor are destroyed. Therefore you should always confirm the shape id is
// valid using Shape.IsValid.
type SensorEndTouchEvent struct {
	// The id of the sensor shape.
	// Warning: this shape may have been destroyed. See Shape.IsValid.
	SensorShapeID ShapeID

	// The id of the shape that stopped touching the sensor shape.
	// Warning: this shape may have been destroyed. See Shape.IsValid.
	VisitorShapeID ShapeID
}

// SensorEvents are buffered in the world and are available as begin/end
// overlap event slices after the time step is complete
// (upstream b2SensorEvents).
// Note: these may become invalid if bodies and/or shapes are destroyed.
//
// Deviation from upstream: the pointer+count pairs become slices.
type SensorEvents struct {
	// Sensor begin touch events
	BeginEvents []SensorBeginTouchEvent

	// Sensor end touch events
	EndEvents []SensorEndTouchEvent
}

// ContactBeginTouchEvent is generated when two shapes begin touching
// (upstream b2ContactBeginTouchEvent).
type ContactBeginTouchEvent struct {
	// Id of the first shape
	ShapeIDA ShapeID

	// Id of the second shape
	ShapeIDB ShapeID

	// The transient contact id. This id is valid until the world is modified
	// or simulated (upstream b2ContactId contactId).
	ContactID ContactID
}

// ContactEndTouchEvent is generated when two shapes stop touching
// (upstream b2ContactEndTouchEvent).
// You will get an end event if you do anything that destroys contacts previous
// to the last world step. These include things like setting the transform,
// destroying a body or shape, or changing a filter or body type.
type ContactEndTouchEvent struct {
	// Id of the first shape.
	// Warning: this shape may have been destroyed. See Shape.IsValid.
	ShapeIDA ShapeID

	// Id of the second shape.
	// Warning: this shape may have been destroyed. See Shape.IsValid.
	ShapeIDB ShapeID

	// Id of the contact (upstream b2ContactId contactId).
	ContactID ContactID
}

// ContactHitEvent is generated when two shapes collide with a speed faster
// than the hit speed threshold (upstream b2ContactHitEvent).
// This may be reported for speculative contacts that have a confirmed impulse.
type ContactHitEvent struct {
	// Id of the first shape
	ShapeIDA ShapeID

	// Id of the second shape
	ShapeIDB ShapeID

	// Id of the contact (upstream b2ContactId contactId).
	ContactID ContactID

	// Point where the shapes hit at the beginning of the time step.
	// This is a mid-point between the two surfaces. It could be at speculative
	// point where the two shapes were not touching at the beginning of the time step.
	Point Vec2

	// Normal vector pointing from shape A to shape B
	Normal Vec2

	// The speed the shapes are approaching. Always positive. Typically in meters per second.
	ApproachSpeed float64
}

// ContactEvents are buffered in the world and are available as event slices
// after the time step is complete (upstream b2ContactEvents).
// Note: these may become invalid if bodies and/or shapes are destroyed.
//
// Deviation from upstream: the pointer+count pairs become slices.
type ContactEvents struct {
	// Begin touch events
	BeginEvents []ContactBeginTouchEvent

	// End touch events
	EndEvents []ContactEndTouchEvent

	// Hit events
	HitEvents []ContactHitEvent
}

// BodyMoveEvent is triggered when a body moves due to simulation
// (upstream b2BodyMoveEvent). Not reported for bodies moved by the user.
// This also has a flag to indicate that the body went to sleep so the
// application can also sleep that actor/entity/object associated with the
// body. On the other hand if the flag does not indicate the body went to sleep
// then the application can treat the actor/entity/object associated with the
// body as awake.
// This is an efficient way for an application to update game object transforms
// rather than calling functions such as Body.GetTransform because this data is
// delivered as a contiguous slice and it is only populated with bodies that
// have moved.
//
// Note: if sleeping is disabled all dynamic and kinematic bodies will trigger
// move events.
type BodyMoveEvent struct {
	// User data. Deviation from upstream: the C void* becomes a uint64 so the
	// ECS wrapper can pack an entity id directly.
	UserData   uint64
	Transform  Transform
	BodyID     BodyID
	FellAsleep bool
}

// BodyEvents are buffered in the world and are available as event slices after
// the time step is complete (upstream b2BodyEvents).
// Note: this data becomes invalid if bodies are destroyed.
//
// Deviation from upstream: the pointer+count pair becomes a slice.
type BodyEvents struct {
	// Move events
	MoveEvents []BodyMoveEvent
}

// JointEvent reports a joint that is awake and has a force and/or torque
// exceeding the threshold (upstream b2JointEvent). The observed forces and
// torques are not returned for efficiency reasons.
type JointEvent struct {
	// The joint id
	JointID JointID

	// The user data from the joint for convenience. Deviation from upstream:
	// the C void* becomes a uint64 so the ECS wrapper can pack an entity id
	// directly.
	UserData uint64
}

// JointEvents are buffered in the world and are available as event slices
// after the time step is complete (upstream b2JointEvents).
// Note: this data becomes invalid if joints are destroyed.
//
// Deviation from upstream: the pointer+count pair becomes a slice.
type JointEvents struct {
	// Joint events
	JointEvents []JointEvent
}

// ContactData is the contact data for two shapes (upstream b2ContactData). By
// convention the manifold normal points from shape A to shape B.
// See Shape.GetContactData and Body.GetContactData.
type ContactData struct {
	// Id of the contact (upstream b2ContactId contactId).
	ContactID ContactID

	ShapeIDA ShapeID
	ShapeIDB ShapeID

	// The manifold of the contact. By convention the normal points from
	// shape A to shape B (upstream b2Manifold manifold).
	Manifold Manifold
}

// CustomFilterFcn is the prototype for a contact filter callback
// (upstream b2CustomFilterFcn).
// This is called when a contact pair is considered for collision. This allows
// you to perform custom logic to prevent collision between shapes. This is
// only called if one of the two shapes has custom filtering enabled.
// Notes:
//   - this is only called if one of the two shapes has enabled custom filtering
//   - this may be called for awake dynamic bodies and sensors
//
// Return false if you want to disable the collision. See ShapeDef.
//
// Warning: do not attempt to modify the world inside this callback.
type CustomFilterFcn func(shapeIDA, shapeIDB ShapeID, ctx any) bool

// PreSolveFcn is the prototype for a pre-solve callback
// (upstream b2PreSolveFcn).
// This is called after a contact is updated. This allows you to inspect a
// contact before it goes to the solver. If you are careful, you can modify the
// contact manifold (e.g. modify the normal).
// Notes:
//   - this is only called if the shape has enabled pre-solve events
//   - this is called only for awake dynamic bodies
//   - this is not called for sensors
//   - the supplied manifold has impulse values from the previous step
//
// Return false if you want to disable the contact this step.
//
// Warning: do not attempt to modify the world inside this callback.
type PreSolveFcn func(shapeIDA, shapeIDB ShapeID, point, normal Vec2, ctx any) bool

// OverlapResultFcn is the prototype callback for overlap queries
// (upstream b2OverlapResultFcn).
// Called for each shape found in the query. Return false to terminate the
// query. See World.OverlapAABB.
type OverlapResultFcn func(shapeID ShapeID, ctx any) bool

// CastResultFcn is the prototype callback for ray and shape casts
// (upstream b2CastResultFcn).
// Called for each shape found in the query. You control how the ray cast
// proceeds by returning a float:
//
//	return -1: ignore this shape and continue
//	return 0: terminate the ray cast
//	return fraction: clip the ray to this point
//	return 1: don't clip the ray and continue
//
// A cast with initial overlap will return a zero fraction and a zero normal.
// See World.CastRay.
//
// shapeID is the shape hit by the ray, point the point of initial
// intersection, normal the normal vector at the point of intersection (zero
// for a shape cast with initial overlap), fraction the fraction along the ray
// at the point of intersection (zero for a shape cast with initial overlap),
// and ctx the user context.
type CastResultFcn func(shapeID ShapeID, point, normal Vec2, fraction float64, ctx any) float64

// PlaneResultFcn is the prototype callback for character movers
// (upstream b2PlaneResultFcn).
// Called for each shape found in the query. Return true to continue gathering
// planes. See World.CollideMover.
type PlaneResultFcn func(shapeID ShapeID, plane *PlaneResult, ctx any) bool

// HexColor holds colors used for debug draw. They mostly match the named SVG
// colors (upstream b2HexColor).
// See https://www.rapidtables.com/web/color/index.html
// https://johndecember.com/html/spec/colorsvg.html
type HexColor int32

const (
	ColorAliceBlue            HexColor = 0xF0F8FF
	ColorAntiqueWhite         HexColor = 0xFAEBD7
	ColorAqua                 HexColor = 0x00FFFF
	ColorAquamarine           HexColor = 0x7FFFD4
	ColorAzure                HexColor = 0xF0FFFF
	ColorBeige                HexColor = 0xF5F5DC
	ColorBisque               HexColor = 0xFFE4C4
	ColorBlack                HexColor = 0x000000
	ColorBlanchedAlmond       HexColor = 0xFFEBCD
	ColorBlue                 HexColor = 0x0000FF
	ColorBlueViolet           HexColor = 0x8A2BE2
	ColorBrown                HexColor = 0xA52A2A
	ColorBurlywood            HexColor = 0xDEB887
	ColorCadetBlue            HexColor = 0x5F9EA0
	ColorChartreuse           HexColor = 0x7FFF00
	ColorChocolate            HexColor = 0xD2691E
	ColorCoral                HexColor = 0xFF7F50
	ColorCornflowerBlue       HexColor = 0x6495ED
	ColorCornsilk             HexColor = 0xFFF8DC
	ColorCrimson              HexColor = 0xDC143C
	ColorCyan                 HexColor = 0x00FFFF
	ColorDarkBlue             HexColor = 0x00008B
	ColorDarkCyan             HexColor = 0x008B8B
	ColorDarkGoldenRod        HexColor = 0xB8860B
	ColorDarkGray             HexColor = 0xA9A9A9
	ColorDarkGreen            HexColor = 0x006400
	ColorDarkKhaki            HexColor = 0xBDB76B
	ColorDarkMagenta          HexColor = 0x8B008B
	ColorDarkOliveGreen       HexColor = 0x556B2F
	ColorDarkOrange           HexColor = 0xFF8C00
	ColorDarkOrchid           HexColor = 0x9932CC
	ColorDarkRed              HexColor = 0x8B0000
	ColorDarkSalmon           HexColor = 0xE9967A
	ColorDarkSeaGreen         HexColor = 0x8FBC8F
	ColorDarkSlateBlue        HexColor = 0x483D8B
	ColorDarkSlateGray        HexColor = 0x2F4F4F
	ColorDarkTurquoise        HexColor = 0x00CED1
	ColorDarkViolet           HexColor = 0x9400D3
	ColorDeepPink             HexColor = 0xFF1493
	ColorDeepSkyBlue          HexColor = 0x00BFFF
	ColorDimGray              HexColor = 0x696969
	ColorDodgerBlue           HexColor = 0x1E90FF
	ColorFireBrick            HexColor = 0xB22222
	ColorFloralWhite          HexColor = 0xFFFAF0
	ColorForestGreen          HexColor = 0x228B22
	ColorFuchsia              HexColor = 0xFF00FF
	ColorGainsboro            HexColor = 0xDCDCDC
	ColorGhostWhite           HexColor = 0xF8F8FF
	ColorGold                 HexColor = 0xFFD700
	ColorGoldenRod            HexColor = 0xDAA520
	ColorGray                 HexColor = 0x808080
	ColorGreen                HexColor = 0x008000
	ColorGreenYellow          HexColor = 0xADFF2F
	ColorHoneyDew             HexColor = 0xF0FFF0
	ColorHotPink              HexColor = 0xFF69B4
	ColorIndianRed            HexColor = 0xCD5C5C
	ColorIndigo               HexColor = 0x4B0082
	ColorIvory                HexColor = 0xFFFFF0
	ColorKhaki                HexColor = 0xF0E68C
	ColorLavender             HexColor = 0xE6E6FA
	ColorLavenderBlush        HexColor = 0xFFF0F5
	ColorLawnGreen            HexColor = 0x7CFC00
	ColorLemonChiffon         HexColor = 0xFFFACD
	ColorLightBlue            HexColor = 0xADD8E6
	ColorLightCoral           HexColor = 0xF08080
	ColorLightCyan            HexColor = 0xE0FFFF
	ColorLightGoldenRodYellow HexColor = 0xFAFAD2
	ColorLightGray            HexColor = 0xD3D3D3
	ColorLightGreen           HexColor = 0x90EE90
	ColorLightPink            HexColor = 0xFFB6C1
	ColorLightSalmon          HexColor = 0xFFA07A
	ColorLightSeaGreen        HexColor = 0x20B2AA
	ColorLightSkyBlue         HexColor = 0x87CEFA
	ColorLightSlateGray       HexColor = 0x778899
	ColorLightSteelBlue       HexColor = 0xB0C4DE
	ColorLightYellow          HexColor = 0xFFFFE0
	ColorLime                 HexColor = 0x00FF00
	ColorLimeGreen            HexColor = 0x32CD32
	ColorLinen                HexColor = 0xFAF0E6
	ColorMagenta              HexColor = 0xFF00FF
	ColorMaroon               HexColor = 0x800000
	ColorMediumAquaMarine     HexColor = 0x66CDAA
	ColorMediumBlue           HexColor = 0x0000CD
	ColorMediumOrchid         HexColor = 0xBA55D3
	ColorMediumPurple         HexColor = 0x9370DB
	ColorMediumSeaGreen       HexColor = 0x3CB371
	ColorMediumSlateBlue      HexColor = 0x7B68EE
	ColorMediumSpringGreen    HexColor = 0x00FA9A
	ColorMediumTurquoise      HexColor = 0x48D1CC
	ColorMediumVioletRed      HexColor = 0xC71585
	ColorMidnightBlue         HexColor = 0x191970
	ColorMintCream            HexColor = 0xF5FFFA
	ColorMistyRose            HexColor = 0xFFE4E1
	ColorMoccasin             HexColor = 0xFFE4B5
	ColorNavajoWhite          HexColor = 0xFFDEAD
	ColorNavy                 HexColor = 0x000080
	ColorOldLace              HexColor = 0xFDF5E6
	ColorOlive                HexColor = 0x808000
	ColorOliveDrab            HexColor = 0x6B8E23
	ColorOrange               HexColor = 0xFFA500
	ColorOrangeRed            HexColor = 0xFF4500
	ColorOrchid               HexColor = 0xDA70D6
	ColorPaleGoldenRod        HexColor = 0xEEE8AA
	ColorPaleGreen            HexColor = 0x98FB98
	ColorPaleTurquoise        HexColor = 0xAFEEEE
	ColorPaleVioletRed        HexColor = 0xDB7093
	ColorPapayaWhip           HexColor = 0xFFEFD5
	ColorPeachPuff            HexColor = 0xFFDAB9
	ColorPeru                 HexColor = 0xCD853F
	ColorPink                 HexColor = 0xFFC0CB
	ColorPlum                 HexColor = 0xDDA0DD
	ColorPowderBlue           HexColor = 0xB0E0E6
	ColorPurple               HexColor = 0x800080
	ColorRebeccaPurple        HexColor = 0x663399
	ColorRed                  HexColor = 0xFF0000
	ColorRosyBrown            HexColor = 0xBC8F8F
	ColorRoyalBlue            HexColor = 0x4169E1
	ColorSaddleBrown          HexColor = 0x8B4513
	ColorSalmon               HexColor = 0xFA8072
	ColorSandyBrown           HexColor = 0xF4A460
	ColorSeaGreen             HexColor = 0x2E8B57
	ColorSeaShell             HexColor = 0xFFF5EE
	ColorSienna               HexColor = 0xA0522D
	ColorSilver               HexColor = 0xC0C0C0
	ColorSkyBlue              HexColor = 0x87CEEB
	ColorSlateBlue            HexColor = 0x6A5ACD
	ColorSlateGray            HexColor = 0x708090
	ColorSnow                 HexColor = 0xFFFAFA
	ColorSpringGreen          HexColor = 0x00FF7F
	ColorSteelBlue            HexColor = 0x4682B4
	ColorTan                  HexColor = 0xD2B48C
	ColorTeal                 HexColor = 0x008080
	ColorThistle              HexColor = 0xD8BFD8
	ColorTomato               HexColor = 0xFF6347
	ColorTurquoise            HexColor = 0x40E0D0
	ColorViolet               HexColor = 0xEE82EE
	ColorWheat                HexColor = 0xF5DEB3
	ColorWhite                HexColor = 0xFFFFFF
	ColorWhiteSmoke           HexColor = 0xF5F5F5
	ColorYellow               HexColor = 0xFFFF00
	ColorYellowGreen          HexColor = 0x9ACD32

	ColorBox2DRed    HexColor = 0xDC3132
	ColorBox2DBlue   HexColor = 0x30AEBF
	ColorBox2DGreen  HexColor = 0x8CC924
	ColorBox2DYellow HexColor = 0xFFEE8C
)

// ContactDrawType is the type of contact point drawing
// (upstream b2ContactDrawType).
type ContactDrawType int32

const (
	DrawContactsNone    ContactDrawType = 0
	DrawContactsClip    ContactDrawType = 1
	DrawContactsAnchorA ContactDrawType = 2
	DrawContactsAnchorB ContactDrawType = 3
	DrawContactsAverage ContactDrawType = 4
)

// DebugDraw holds callbacks you can implement to draw a Box2D world
// (upstream b2DebugDraw). This structure should be zero initialized.
//
// Deviation from upstream: the vertices pointer+count pairs become slices.
type DebugDraw struct {
	// DrawPolygonFcn draws a closed polygon provided in CCW order.
	DrawPolygonFcn func(vertices []Vec2, color HexColor, ctx any)

	// DrawSolidPolygonFcn draws a solid closed polygon provided in CCW order.
	DrawSolidPolygonFcn func(transform Transform, vertices []Vec2, radius float64, color HexColor, ctx any)

	// DrawCircleFcn draws a circle.
	DrawCircleFcn func(center Vec2, radius float64, color HexColor, ctx any)

	// DrawSolidCircleFcn draws a solid circle.
	DrawSolidCircleFcn func(transform Transform, radius float64, color HexColor, ctx any)

	// DrawSolidCapsuleFcn draws a solid capsule.
	DrawSolidCapsuleFcn func(p1, p2 Vec2, radius float64, color HexColor, ctx any)

	// DrawLineFcn draws a line segment.
	DrawLineFcn func(p1, p2 Vec2, color HexColor, ctx any)

	// DrawTransformFcn draws a transform. Choose your own length scale.
	DrawTransformFcn func(transform Transform, ctx any)

	// DrawPointFcn draws a point.
	DrawPointFcn func(p Vec2, size float64, color HexColor, ctx any)

	// DrawStringFcn draws a string in world space.
	DrawStringFcn func(p Vec2, s string, color HexColor, ctx any)

	// World bounds to use for debug draw
	DrawingBounds AABB

	// Scale to use when drawing forces
	ForceScale float64

	// Global scaling for joint drawing
	JointScale float64

	// Option to draw contact points
	ContactDrawType ContactDrawType

	// Option to draw shapes
	DrawShapes bool

	// Option to draw joints
	DrawJoints bool

	// Option to draw additional information for joints
	DrawJointExtras bool

	// Option to draw the bounding boxes for shapes
	DrawBounds bool

	// Option to draw the mass and center of mass of dynamic bodies
	DrawMass bool

	// Option to draw body names
	DrawBodyNames bool

	// Option to visualize the graph coloring used for contacts and joints
	DrawGraphColors bool

	// Option to draw contact feature ids
	DrawContactFeatures bool

	// Option to draw contact normals
	DrawContactNormals bool

	// Option to draw contact normal forces
	DrawContactForces bool

	// Option to draw contact friction forces
	DrawFrictionForces bool

	// Option to draw islands as bounding boxes
	DrawIslands bool

	// User context that is passed as an argument to drawing callback functions
	Context any
}

// ---------------------------------------------------------------------------
// src/types.c — default constructors
// ---------------------------------------------------------------------------

// DefaultWorldDef initializes a world definition
// (upstream b2DefaultWorldDef).
func DefaultWorldDef() WorldDef {
	lengthUnits := GetLengthUnitsPerMeter()
	var def WorldDef
	def.Gravity.X = 0.0
	def.Gravity.Y = -10.0
	def.HitEventThreshold = 1.0 * lengthUnits
	def.RestitutionThreshold = 1.0 * lengthUnits
	def.ContactSpeed = 3.0 * lengthUnits
	def.ContactHertz = 30.0
	def.ContactDampingRatio = 10.0

	// 400 meters per second, faster than the speed of sound
	def.MaximumLinearSpeed = 400.0 * lengthUnits
	def.EnableSleep = true
	def.EnableContinuous = true
	def.initialized = true
	return def
}

// DefaultBodyDef initializes a body definition (upstream b2DefaultBodyDef).
func DefaultBodyDef() BodyDef {
	var def BodyDef
	def.Type = StaticBody
	def.Rotation = RotIdentity
	def.SleepThreshold = 0.05 * GetLengthUnitsPerMeter()
	def.GravityScale = 1.0
	def.EnableSleep = true
	def.IsAwake = true
	def.IsEnabled = true
	def.initialized = true
	return def
}

// DefaultFilter initializes a collision filter (upstream b2DefaultFilter).
func DefaultFilter() Filter {
	return Filter{
		CategoryBits: DefaultCategoryBits,
		MaskBits:     DefaultMaskBits,
		GroupIndex:   0,
	}
}

// DefaultQueryFilter initializes a query filter
// (upstream b2DefaultQueryFilter).
func DefaultQueryFilter() QueryFilter {
	return QueryFilter{
		CategoryBits: DefaultCategoryBits,
		MaskBits:     DefaultMaskBits,
	}
}

// DefaultShapeDef initializes a shape definition (upstream b2DefaultShapeDef).
func DefaultShapeDef() ShapeDef {
	var def ShapeDef
	def.Material.Friction = 0.6
	def.Density = 1.0
	def.Filter = DefaultFilter()
	def.UpdateBodyMass = true
	def.InvokeContactCreation = true
	def.initialized = true
	return def
}

// DefaultSurfaceMaterial initializes a surface material
// (upstream b2DefaultSurfaceMaterial).
func DefaultSurfaceMaterial() SurfaceMaterial {
	return SurfaceMaterial{
		Friction: 0.6,
	}
}

// DefaultChainDef initializes a chain definition (upstream b2DefaultChainDef).
//
// Deviation from upstream: upstream points materials at a shared static
// default material; this allocates a fresh one-element slice per call so the
// caller cannot mutate a package-level value.
func DefaultChainDef() ChainDef {
	var def ChainDef
	def.Materials = []SurfaceMaterial{{Friction: 0.6}}
	def.Filter = DefaultFilter()
	def.initialized = true
	return def
}

func emptyDrawPolygon(_ []Vec2, _ HexColor, _ any) {}

func emptyDrawSolidPolygon(_ Transform, _ []Vec2, _ float64, _ HexColor, _ any) {}

func emptyDrawCircle(_ Vec2, _ float64, _ HexColor, _ any) {}

func emptyDrawSolidCircle(_ Transform, _ float64, _ HexColor, _ any) {}

func emptyDrawSolidCapsule(_, _ Vec2, _ float64, _ HexColor, _ any) {}

func emptyDrawSegment(_, _ Vec2, _ HexColor, _ any) {}

func emptyDrawTransform(_ Transform, _ any) {}

func emptyDrawPoint(_ Vec2, _ float64, _ HexColor, _ any) {}

func emptyDrawString(_ Vec2, _ string, _ HexColor, _ any) {}

// DefaultDebugDraw initializes a drawing interface
// (upstream b2DefaultDebugDraw). This allows you to implement a sub-set of the
// drawing functions.
//
// The drawing bounds keep the upstream FLT_MAX magnitude even though this port
// computes in float64.
func DefaultDebugDraw() DebugDraw {
	var draw DebugDraw

	// These allow the user to skip some implementations and not hit nil calls.
	draw.DrawPolygonFcn = emptyDrawPolygon
	draw.DrawSolidPolygonFcn = emptyDrawSolidPolygon
	draw.DrawCircleFcn = emptyDrawCircle
	draw.DrawSolidCircleFcn = emptyDrawSolidCircle
	draw.DrawSolidCapsuleFcn = emptyDrawSolidCapsule
	draw.DrawLineFcn = emptyDrawSegment
	draw.DrawTransformFcn = emptyDrawTransform
	draw.DrawPointFcn = emptyDrawPoint
	draw.DrawStringFcn = emptyDrawString

	draw.DrawingBounds.LowerBound = Vec2{-math.MaxFloat32, -math.MaxFloat32}
	draw.DrawingBounds.UpperBound = Vec2{math.MaxFloat32, math.MaxFloat32}
	draw.ForceScale = 1.0
	draw.JointScale = 1.0
	draw.DrawShapes = true

	return draw
}
