package harness

import (
	"fmt"
	"math"
	"testing"

	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/probe"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// LaneWidth is the world-space X spacing between scenarios. Every scenario is
// laid out in its own lane so bodies from unrelated scenarios can never touch,
// collide, or show up in each other's raycasts and AABB overlaps. Scenario code
// is written in lane-local coordinates; Ctx translates on the way in and out.
const LaneWidth = 300.0

// ProbeRow is the archetype every harness-spawned body uses.
type ProbeRow struct {
	Probe     cardinal.Ref[probe.Probe]
	Transform cardinal.Ref[physics.Transform2D]
	Velocity  cardinal.Ref[physics.Velocity2D]
	Body      cardinal.Ref[physics.PhysicsBody2D]
}

// Probes is the search over every harness body. Contains (not Exact) so a
// scenario is free to add extra components to an entity later on.
type Probes = cardinal.Contains[ProbeRow]

// Step is one scheduled action or assertion, run on the given tick after the
// physics pipeline has stepped. Steps sharing a tick run in declaration order.
type Step struct {
	Do   func(c *Ctx)
	Tick uint64
}

// Scenario is one self-contained group of physics checks.
//
// Setup runs on cardinal.Init, before the plugin's own Init system, so the
// initial FullRebuildFromECS sees the bodies. EachTick runs on PreUpdate before
// the physics pipeline (use it to drive gameplay-owned bodies). Steps run on
// Update, after the pipeline, where post-step positions and this tick's contact
// events are both visible.
type Scenario struct {
	Setup    func(c *Ctx)
	EachTick func(c *Ctx)
	Name     string
	Steps    []Step
	lane     float64
}

// LastTick returns the highest tick any of the scenario's steps is scheduled for.
func (s *Scenario) LastTick() uint64 {
	var last uint64
	for _, st := range s.Steps {
		if st.Tick > last {
			last = st.Tick
		}
	}
	return last
}

// EventKind distinguishes the four physics system events.
type EventKind uint8

// Physics system event kinds.
const (
	ContactBegin EventKind = iota
	ContactEnd
	TriggerBegin
	TriggerEnd
)

// String returns the event kind's name as it appears in failure messages.
func (k EventKind) String() string {
	switch k {
	case ContactBegin:
		return "ContactBegin"
	case ContactEnd:
		return "ContactEnd"
	case TriggerBegin:
		return "TriggerBegin"
	case TriggerEnd:
		return "TriggerEnd"
	default:
		return fmt.Sprintf("EventKind(%d)", uint8(k))
	}
}

// LoggedEvent is one physics event captured with the tick it arrived on.
type LoggedEvent struct {
	Payload physics.ContactEventPayload
	Tick    uint64
	Kind    EventKind
}

// Involves reports whether the event is between exactly a and b, in either order.
func (e LoggedEvent) Involves(a, b cardinal.EntityID) bool {
	return (e.Payload.EntityA == a && e.Payload.EntityB == b) ||
		(e.Payload.EntityA == b && e.Payload.EntityB == a)
}

// ShapeIndexFor returns the shape slot the given entity contributed to the
// event, so a compound body's contacts can be traced back to the exact child.
func (e LoggedEvent) ShapeIndexFor(id cardinal.EntityID) (int, bool) {
	switch id {
	case e.Payload.EntityA:
		return e.Payload.ShapeIndexA, true
	case e.Payload.EntityB:
		return e.Payload.ShapeIndexB, true
	default:
		return 0, false
	}
}

// FilterFor returns the fixture filter the given entity contributed to the event.
func (e LoggedEvent) FilterFor(id cardinal.EntityID) (physics.FixtureFilterBits, bool) {
	switch id {
	case e.Payload.EntityA:
		return e.Payload.FilterA, true
	case e.Payload.EntityB:
		return e.Payload.FilterB, true
	default:
		return physics.FixtureFilterBits{}, false
	}
}

// Touches reports whether the event names the given entity on either side.
func (e LoggedEvent) Touches(a cardinal.EntityID) bool {
	return e.Payload.EntityA == a || e.Payload.EntityB == a
}

// Ctx is the API a scenario uses to spawn, inspect, mutate and assert. All
// positions crossing this boundary are lane-local: Ctx adds the lane offset when
// writing to ECS and subtracts it when reading back.
type Ctx struct {
	report     *Report
	probes     *Probes
	events     *eventStore
	plugin     *physics.Plugin
	allowReset func()
	scenario   string
	lane       float64
	tick       uint64
	// tb, when bound, attributes failures to the scenario's own line: every
	// assertion marks itself a helper before reporting.
	tb testing.TB
}

// Plugin returns the physics plugin driving this world. Queries, Reset and
// Engine are methods on it — the package holds no runtime state.
func (c *Ctx) Plugin() *physics.Plugin { return c.plugin }

// ExpectWorldReset silences the runner's "the world disappeared" watchdog for
// the current tick, so a scenario that calls Plugin.Reset can say so.
func (c *Ctx) ExpectWorldReset() {
	if c.allowReset != nil {
		c.allowReset()
	}
}

// Tick returns the tick currently being simulated.
func (c *Ctx) Tick() uint64 { return c.tick }

// Scenario returns the running scenario's name.
func (c *Ctx) Scenario() string { return c.scenario }

// Report exposes the underlying report for the few places that need it directly.
func (c *Ctx) Report() *Report { return c.report }

// Lane returns this scenario's world-space X offset.
func (c *Ctx) Lane() float64 { return c.lane }

func (c *Ctx) toWorld(v physics.Vec2) physics.Vec2 {
	return physics.Vec2{X: v.X + c.lane, Y: v.Y}
}

func (c *Ctx) toLocal(v physics.Vec2) physics.Vec2 {
	return physics.Vec2{X: v.X - c.lane, Y: v.Y}
}

// -----------------------------------------------------------------------------
// Spawning and entity access
// -----------------------------------------------------------------------------

// SpawnFull creates a probe entity from a complete pose, velocity and body.
func (c *Ctx) SpawnFull(
	label string,
	t physics.Transform2D,
	v physics.Velocity2D,
	pb physics.PhysicsBody2D,
) cardinal.EntityID {
	t.Position = c.toWorld(t.Position)
	id, row := c.probes.Create()
	row.Probe.Set(probe.Probe{Scenario: c.scenario, Label: label})
	row.Transform.Set(t)
	row.Velocity.Set(v)
	row.Body.Set(pb)
	c.events.own(id, c.scenario, label)
	return id
}

// Spawn creates a probe entity at rest at the given lane-local position.
func (c *Ctx) Spawn(label string, x, y float64, pb physics.PhysicsBody2D) cardinal.EntityID {
	return c.SpawnFull(label,
		physics.Transform2D{Position: physics.Vec2{X: x, Y: y}},
		physics.Velocity2D{},
		pb,
	)
}

// SpawnMoving creates a probe entity with an initial linear velocity.
func (c *Ctx) SpawnMoving(
	label string, x, y, vx, vy float64, pb physics.PhysicsBody2D,
) cardinal.EntityID {
	return c.SpawnFull(label,
		physics.Transform2D{Position: physics.Vec2{X: x, Y: y}},
		physics.Velocity2D{Linear: physics.Vec2{X: vx, Y: vy}},
		pb,
	)
}

// SpawnSpinning creates a probe entity with an initial angular velocity.
func (c *Ctx) SpawnSpinning(
	label string, x, y, angular float64, pb physics.PhysicsBody2D,
) cardinal.EntityID {
	return c.SpawnFull(label,
		physics.Transform2D{Position: physics.Vec2{X: x, Y: y}},
		physics.Velocity2D{Angular: angular},
		pb,
	)
}

// Label returns the human-readable label recorded for an entity at spawn time.
// It keeps working after the entity is destroyed, which matters for end events.
func (c *Ctx) Label(id cardinal.EntityID) string { return c.events.label(id) }

// Alive reports whether the entity still exists with the probe archetype.
func (c *Ctx) Alive(id cardinal.EntityID) bool {
	_, err := c.probes.GetByID(id)
	return err == nil
}

// Pos returns the entity's lane-local position, or the zero vector if it is gone.
func (c *Ctx) Pos(id cardinal.EntityID) physics.Vec2 {
	row, err := c.probes.GetByID(id)
	if err != nil {
		return physics.Vec2{}
	}
	return c.toLocal(row.Transform.Get().Position)
}

// Rot returns the entity's rotation in radians.
func (c *Ctx) Rot(id cardinal.EntityID) float64 {
	row, err := c.probes.GetByID(id)
	if err != nil {
		return 0
	}
	return row.Transform.Get().Rotation
}

// Vel returns the entity's linear velocity.
func (c *Ctx) Vel(id cardinal.EntityID) physics.Vec2 {
	row, err := c.probes.GetByID(id)
	if err != nil {
		return physics.Vec2{}
	}
	return row.Velocity.Get().Linear
}

// AngVel returns the entity's angular velocity.
func (c *Ctx) AngVel(id cardinal.EntityID) float64 {
	row, err := c.probes.GetByID(id)
	if err != nil {
		return 0
	}
	return row.Velocity.Get().Angular
}

// Speed returns the magnitude of the entity's linear velocity.
func (c *Ctx) Speed(id cardinal.EntityID) float64 {
	v := c.Vel(id)
	return math.Hypot(v.X, v.Y)
}

// Body returns a deep copy of the entity's PhysicsBody2D. The Shapes slice is
// cloned so a scenario can edit it without aliasing ECS storage; write it back
// with SetBody.
func (c *Ctx) Body(id cardinal.EntityID) physics.PhysicsBody2D {
	row, err := c.probes.GetByID(id)
	if err != nil {
		return physics.PhysicsBody2D{}
	}
	return CloneBody(row.Body.Get())
}

// SetPos moves the entity to a lane-local position.
func (c *Ctx) SetPos(id cardinal.EntityID, x, y float64) {
	row, err := c.probes.GetByID(id)
	if err != nil {
		return
	}
	t := row.Transform.Get()
	t.Position = c.toWorld(physics.Vec2{X: x, Y: y})
	row.Transform.Set(t)
}

// SetRot sets the entity's rotation in radians.
func (c *Ctx) SetRot(id cardinal.EntityID, radians float64) {
	row, err := c.probes.GetByID(id)
	if err != nil {
		return
	}
	t := row.Transform.Get()
	t.Rotation = radians
	row.Transform.Set(t)
}

// SetVel sets the entity's linear velocity.
func (c *Ctx) SetVel(id cardinal.EntityID, vx, vy float64) {
	row, err := c.probes.GetByID(id)
	if err != nil {
		return
	}
	v := row.Velocity.Get()
	v.Linear = physics.Vec2{X: vx, Y: vy}
	row.Velocity.Set(v)
}

// SetAngVel sets the entity's angular velocity.
func (c *Ctx) SetAngVel(id cardinal.EntityID, angular float64) {
	row, err := c.probes.GetByID(id)
	if err != nil {
		return
	}
	v := row.Velocity.Get()
	v.Angular = angular
	row.Velocity.Set(v)
}

// SetBody replaces the entity's PhysicsBody2D.
func (c *Ctx) SetBody(id cardinal.EntityID, pb physics.PhysicsBody2D) {
	row, err := c.probes.GetByID(id)
	if err != nil {
		return
	}
	row.Body.Set(pb)
}

// EditBody reads the body, hands it to edit, and writes the result back.
func (c *Ctx) EditBody(id cardinal.EntityID, edit func(pb *physics.PhysicsBody2D)) {
	pb := c.Body(id)
	edit(&pb)
	c.SetBody(id, pb)
}

// Destroy removes the entity from the world.
func (c *Ctx) Destroy(id cardinal.EntityID) bool { return c.probes.Destroy(id) }

// CloneBody deep-copies a PhysicsBody2D including its shapes and their slice
// geometry, so edits to the copy cannot reach the original.
func CloneBody(pb physics.PhysicsBody2D) physics.PhysicsBody2D {
	out := pb
	out.Shapes = make([]physics.ColliderShape, len(pb.Shapes))
	for i, s := range pb.Shapes {
		s.Vertices = append([]physics.Vec2(nil), s.Vertices...)
		s.ChainPoints = append([]physics.Vec2(nil), s.ChainPoints...)
		out.Shapes[i] = s
	}
	return out
}

// -----------------------------------------------------------------------------
// Queries (lane-local in, lane-local out)
// -----------------------------------------------------------------------------

// Raycast casts from one lane-local point to another. The returned hit point is
// translated back into lane-local space.
func (c *Ctx) Raycast(fromX, fromY, toX, toY float64, filter *physics.Filter) physics.RaycastResult {
	res := c.plugin.Raycast(physics.RaycastRequest{
		Origin: c.toWorld(physics.Vec2{X: fromX, Y: fromY}),
		End:    c.toWorld(physics.Vec2{X: toX, Y: toY}),
		Filter: filter,
	})
	if res.Hit {
		res.Point = c.toLocal(res.Point)
	}
	return res
}

// OverlapAABB queries a lane-local axis-aligned box.
func (c *Ctx) OverlapAABB(minX, minY, maxX, maxY float64, filter *physics.Filter) physics.AABBOverlapResult {
	return c.plugin.OverlapAABB(physics.AABBOverlapRequest{
		Min:    c.toWorld(physics.Vec2{X: minX, Y: minY}),
		Max:    c.toWorld(physics.Vec2{X: maxX, Y: maxY}),
		Filter: filter,
	})
}

// CircleSweep sweeps a circle between two lane-local points.
func (c *Ctx) CircleSweep(
	fromX, fromY, toX, toY, radius, maxFraction float64, filter *physics.Filter,
) physics.CircleSweepResult {
	res := c.plugin.CircleSweep(physics.CircleSweepRequest{
		Start:       c.toWorld(physics.Vec2{X: fromX, Y: fromY}),
		End:         c.toWorld(physics.Vec2{X: toX, Y: toY}),
		Radius:      radius,
		Filter:      filter,
		MaxFraction: maxFraction,
	})
	if res.Hit {
		res.Point = c.toLocal(res.Point)
	}
	return res
}

// OverlapHits reports whether an AABB overlap returned the given entity.
func (c *Ctx) OverlapHits(res physics.AABBOverlapResult, id cardinal.EntityID) bool {
	for _, h := range res.Hits {
		if h.Entity == id {
			return true
		}
	}
	return false
}

// OverlapHitsShape reports whether an AABB overlap returned a specific shape slot.
func (c *Ctx) OverlapHitsShape(res physics.AABBOverlapResult, id cardinal.EntityID, shapeIndex int) bool {
	for _, h := range res.Hits {
		if h.Entity == id && h.ShapeIndex == shapeIndex {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------------
// Events
// -----------------------------------------------------------------------------

// Events returns every event of the given kind recorded for this scenario so far.
func (c *Ctx) Events(kind EventKind) []LoggedEvent {
	return c.events.forScenario(c.scenario, kind)
}

// EventsBetween returns this scenario's events of a kind involving exactly a and b.
func (c *Ctx) EventsBetween(kind EventKind, a, b cardinal.EntityID) []LoggedEvent {
	var out []LoggedEvent
	for _, e := range c.events.forScenario(c.scenario, kind) {
		if e.Involves(a, b) {
			out = append(out, e)
		}
	}
	return out
}

// CountBetween counts this scenario's events of a kind involving exactly a and b.
func (c *Ctx) CountBetween(kind EventKind, a, b cardinal.EntityID) int {
	return len(c.EventsBetween(kind, a, b))
}

// CountTouching counts this scenario's events of a kind naming the given entity.
func (c *Ctx) CountTouching(kind EventKind, id cardinal.EntityID) int {
	n := 0
	for _, e := range c.events.forScenario(c.scenario, kind) {
		if e.Touches(id) {
			n++
		}
	}
	return n
}

// EventsOnTick returns this scenario's events of a kind recorded on one tick.
func (c *Ctx) EventsOnTick(kind EventKind, tick uint64) []LoggedEvent {
	var out []LoggedEvent
	for _, e := range c.events.forScenario(c.scenario, kind) {
		if e.Tick == tick {
			out = append(out, e)
		}
	}
	return out
}

// -----------------------------------------------------------------------------
// Assertions
// -----------------------------------------------------------------------------

// True asserts a condition holds.
func (c *Ctx) True(check string, got bool, format string, args ...any) bool {
	if c.tb != nil {
		c.tb.Helper()
	}
	if got {
		c.report.Pass(c.scenario, check, c.tick)
		return true
	}
	c.report.Fail(c.scenario, check, c.tick, format, args...)
	return false
}

// False asserts a condition does not hold.
func (c *Ctx) False(check string, got bool, format string, args ...any) bool {
	if c.tb != nil {
		c.tb.Helper()
	}
	return c.True(check, !got, format, args...)
}

// Near asserts got is within tol of want.
func (c *Ctx) Near(check string, got, want, tol float64) bool {
	if c.tb != nil {
		c.tb.Helper()
	}
	if !finite(got) {
		return c.True(check, false, "got %v (not finite), want %v ±%v", got, want, tol)
	}
	return c.True(check, math.Abs(got-want) <= tol,
		"got %.6f, want %.6f ±%.6f (off by %.6f)", got, want, tol, math.Abs(got-want))
}

// NearVec asserts both components of got are within tol of want.
func (c *Ctx) NearVec(check string, got, want physics.Vec2, tol float64) bool {
	if c.tb != nil {
		c.tb.Helper()
	}
	ok := finite(got.X) && finite(got.Y) &&
		math.Abs(got.X-want.X) <= tol && math.Abs(got.Y-want.Y) <= tol
	return c.True(check, ok, "got (%.6f, %.6f), want (%.6f, %.6f) ±%.6f",
		got.X, got.Y, want.X, want.Y, tol)
}

// Greater asserts got > threshold.
func (c *Ctx) Greater(check string, got, threshold float64) bool {
	if c.tb != nil {
		c.tb.Helper()
	}
	return c.True(check, finite(got) && got > threshold, "got %.6f, want > %.6f", got, threshold)
}

// GreaterEq asserts got >= threshold.
func (c *Ctx) GreaterEq(check string, got, threshold float64) bool {
	if c.tb != nil {
		c.tb.Helper()
	}
	return c.True(check, finite(got) && got >= threshold, "got %.6f, want >= %.6f", got, threshold)
}

// Less asserts got < threshold.
func (c *Ctx) Less(check string, got, threshold float64) bool {
	if c.tb != nil {
		c.tb.Helper()
	}
	return c.True(check, finite(got) && got < threshold, "got %.6f, want < %.6f", got, threshold)
}

// LessEq asserts got <= threshold.
func (c *Ctx) LessEq(check string, got, threshold float64) bool {
	if c.tb != nil {
		c.tb.Helper()
	}
	return c.True(check, finite(got) && got <= threshold, "got %.6f, want <= %.6f", got, threshold)
}

// Between asserts lo <= got <= hi.
func (c *Ctx) Between(check string, got, lo, hi float64) bool {
	if c.tb != nil {
		c.tb.Helper()
	}
	return c.True(check, finite(got) && got >= lo && got <= hi,
		"got %.6f, want within [%.6f, %.6f]", got, lo, hi)
}

// Int asserts an integer equals want.
func (c *Ctx) Int(check string, got, want int) bool {
	if c.tb != nil {
		c.tb.Helper()
	}
	return c.True(check, got == want, "got %d, want %d", got, want)
}

// IntAtLeast asserts an integer is at least want.
func (c *Ctx) IntAtLeast(check string, got, want int) bool {
	if c.tb != nil {
		c.tb.Helper()
	}
	return c.True(check, got >= want, "got %d, want >= %d", got, want)
}

// Str asserts a string equals want.
func (c *Ctx) Str(check, got, want string) bool {
	if c.tb != nil {
		c.tb.Helper()
	}
	return c.True(check, got == want, "got %q, want %q", got, want)
}

// NoError asserts err is nil.
func (c *Ctx) NoError(check string, err error) bool {
	if c.tb != nil {
		c.tb.Helper()
	}
	return c.True(check, err == nil, "unexpected error: %v", err)
}

// HasError asserts err is non-nil.
func (c *Ctx) HasError(check string, err error) bool {
	if c.tb != nil {
		c.tb.Helper()
	}
	return c.True(check, err != nil, "expected an error, got nil")
}

// Note records a diagnostic line without affecting pass/fail counts.
func (c *Ctx) Note(format string, args ...any) {
	if c.tb != nil {
		c.tb.Helper()
	}
	c.report.Note(c.scenario, c.tick, format, args...)
}

// Skip records that a check could not run.
func (c *Ctx) Skip(check, format string, args ...any) {
	if c.tb != nil {
		c.tb.Helper()
	}
	c.report.Skip(c.scenario, check, c.tick, format, args...)
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
