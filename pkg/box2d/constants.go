// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file include/box2d/constants.h, src/core.h.

package box2d

// NullIndex is B2_NULL_INDEX.
const NullIndex = -1

// SecretCookie is B2_SECRET_COOKIE.
const SecretCookie = 1152023

// Alignment is B2_ALIGNMENT.
const Alignment = 32

// Huge is B2_HUGE (100000.0f * b2GetLengthUnitsPerMeter()).
const Huge = 100000.0

// MaxWorkers is B2_MAX_WORKERS.
const MaxWorkers = 64

// GraphColorCount is B2_GRAPH_COLOR_COUNT.
const GraphColorCount = 24

// LinearSlop is B2_LINEAR_SLOP (0.005f * b2GetLengthUnitsPerMeter()).
const LinearSlop = 0.005

// MaxWorlds is B2_MAX_WORLDS.
const MaxWorlds = 128

// MaxRotation is B2_MAX_ROTATION (0.25f * B2_PI).
const MaxRotation = 0.25 * Pi

// SpeculativeDistance is B2_SPECULATIVE_DISTANCE (4.0f * B2_LINEAR_SLOP).
const SpeculativeDistance = 4.0 * LinearSlop

// ContactRecycleDistance is B2_CONTACT_RECYCLE_DISTANCE (10.0f * B2_LINEAR_SLOP).
const ContactRecycleDistance = 10.0 * LinearSlop

// MaxAABBMargin is B2_MAX_AABB_MARGIN (0.05f * b2GetLengthUnitsPerMeter()).
const MaxAABBMargin = 0.05

// AABBMarginFraction is B2_AABB_MARGIN_FRACTION.
const AABBMarginFraction = 0.125

// TimeToSleep is B2_TIME_TO_SLEEP.
const TimeToSleep = 0.5
