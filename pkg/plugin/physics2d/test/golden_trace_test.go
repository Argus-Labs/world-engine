package physics2d_test

// Golden-trace regression harness.
//
// Purpose: pin the observable behavior of the physics backend to committed JSON fixtures under
// testdata/. Structural results (entity sets, event sequences, query hit identity) must match
// EXACTLY; numeric results must match within goldenDelta. Do not loosen the tolerance for
// unrelated changes.
//
// Fixture provenance: the committed fixtures were RECORDED FROM THE CGO Box2D v3.2.0 BACKEND
// before that backend was deleted. They are therefore a cross-backend regression anchor — the
// pure-Go port in pkg/box2d is held against behavior produced by independent (C) code, which is
// exactly what makes this test valuable.
//
// Regenerate fixtures with:
//
//	PHYSICS2D_UPDATE_GOLDEN=1 go test ./pkg/plugin/physics2d/test/ -run TestGoldenTrace -count=1
//
// WARNING: regeneration now records the PURE-GO backend and permanently forfeits the
// cross-backend anchor — the fixtures would then only prove the port agrees with itself. Use the
// generator only with intent (e.g. adding a new scenario, or an approved deliberate behavior
// change), never to make a failing test pass.
//
// Recording cadence: body state is sampled every goldenSampleEvery ticks plus the final tick, to
// keep fixtures small. Contact/trigger events are recorded on EVERY tick (they are sparse).
//
// Determinism notes (see also the per-field comments below):
//   - Floats are stored as strconv.FormatFloat(v, 'g', 17, 64) strings so JSON round-trips the
//     exact bit pattern; the verify path parses them back and compares with require.InDelta.
//   - Bodies are sorted by EntityID before recording; ECS iteration order is not relied upon.
//   - AABB overlap hits are sorted by (entity, shape index); the broadphase traversal order is an
//     implementation detail of the backend and is deliberately NOT part of the contract.
//   - The relative order of the four event kinds within one tick is imposed by this harness
//     (contact_begin, contact_end, trigger_begin, trigger_end). Order WITHIN one kind is the
//     backend's emission order and IS part of the contract.
//   - Verify mode runs t.Parallel (the pure-Go backend is per-instance state); generator mode
//     stays serial while rewriting fixtures.

import (
	"cmp"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

const (
	// goldenSampleEvery is the body-state sampling cadence in ticks. The final tick is always
	// sampled in addition, whether or not it lands on the cadence.
	goldenSampleEvery = 10
	// goldenDelta is the numeric tolerance used in verify mode. The fixtures were recorded on
	// the float32 CGO backend; the pure-Go backend computes in float64, so trajectories drift
	// slightly. Observed max deltas against the committed CGO fixtures: ~3e-5 for most
	// scenarios, ~2e-4 for filter_matrix, ~2.4e-3 for capsule_chain_ground (chain contact
	// stacking is the most rounding-sensitive scene). 1e-2 gives ~4x headroom over the worst
	// case while still catching real behavioral regressions. Structural assertions (entity
	// sets, event sequences incl. ticks, query hit identity) remain exact.
	goldenDelta = 1e-2
	// goldenUpdateEnv, when set to "1", switches the test into generator mode.
	goldenUpdateEnv = "PHYSICS2D_UPDATE_GOLDEN"
)

// ---------------------------------------------------------------------------
// Fixture schema
// ---------------------------------------------------------------------------

// goldenTrace is the committed reference behavior of a single scenario.
type goldenTrace struct {
	Scenario  string        `json:"scenario"`
	TickCount int           `json:"tick_count"`
	Frames    []goldenFrame `json:"frames"`
	Events    []goldenEvent `json:"events"`
	Queries   []goldenQuery `json:"queries"`
}

// goldenFrame is one sampled tick of body state.
type goldenFrame struct {
	Tick   int          `json:"tick"`
	Bodies []goldenBody `json:"bodies"`
}

// goldenBody is one physics entity's pose and motion at a sampled tick. All numeric fields are
// 'g'-17 formatted float64 strings for exact JSON round-tripping.
type goldenBody struct {
	Entity   uint32 `json:"entity"`
	Role     string `json:"role"`
	PosX     string `json:"pos_x"`
	PosY     string `json:"pos_y"`
	Rotation string `json:"rotation"`
	VelX     string `json:"vel_x"`
	VelY     string `json:"vel_y"`
	AngVel   string `json:"ang_vel"`
}

// goldenEvent is one contact or trigger begin/end event, in emission order within its kind.
type goldenEvent struct {
	Tick        int    `json:"tick"`
	Kind        string `json:"kind"`
	Sensor      bool   `json:"sensor"`
	EntityA     uint32 `json:"entity_a"`
	EntityB     uint32 `json:"entity_b"`
	ShapeIndexA int    `json:"shape_index_a"`
	ShapeIndexB int    `json:"shape_index_b"`
}

// goldenQueryHit is one (entity, shape) pair from an AABB overlap result.
type goldenQueryHit struct {
	Entity     uint32 `json:"entity"`
	ShapeIndex int    `json:"shape_index"`
}

// goldenQuery is the result of one named query executed at a given tick. Raycast and circle sweep
// populate the scalar hit fields; AABB overlap populates Hits.
type goldenQuery struct {
	Tick       int              `json:"tick"`
	Name       string           `json:"name"`
	Hit        bool             `json:"hit"`
	Entity     uint32           `json:"entity"`
	ShapeIndex int              `json:"shape_index"`
	PointX     string           `json:"point_x"`
	PointY     string           `json:"point_y"`
	NormalX    string           `json:"normal_x"`
	NormalY    string           `json:"normal_y"`
	Fraction   string           `json:"fraction"`
	Hits       []goldenQueryHit `json:"hits"`
}

// ---------------------------------------------------------------------------
// Float encoding
// ---------------------------------------------------------------------------

// goldenFloat renders v so that ParseFloat returns exactly v.
func goldenFloat(v float64) string { return strconv.FormatFloat(v, 'g', 17, 64) }

func goldenParse(t *testing.T, s, what string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	require.NoErrorf(t, err, "malformed float in fixture field %s: %q", what, s)
	return v
}

// ---------------------------------------------------------------------------
// Scenario definition and runner
// ---------------------------------------------------------------------------

// goldenScenario describes one recorded simulation.
type goldenScenario struct {
	name      string
	gravity   physics.Vec2
	tickCount int
	// setup registers the entity-spawning system (Init hook) on the world.
	setup func(w *cardinal.World)
	// queries, when non-nil, is called once per tick after the physics step. It returns the query
	// samples to record for that tick (empty slice for ticks with no queries). Tick is filled in
	// by the runner.
	queries func(p *physics.Plugin, tick int) []goldenQuery
}

// runGoldenScenario builds the world, registers the recorder, runs tickCount ticks and returns
// the recorded trace. Workers stays at the serial default.
func runGoldenScenario(t *testing.T, sc goldenScenario) goldenTrace {
	t.Helper()
	return runGoldenScenarioWorkers(t, sc, 0)
}

// runGoldenScenarioWorkers is runGoldenScenario with an explicit
// physics.Config.Workers value (see TestGoldenTraceWorkers, workers_test.go).
func runGoldenScenarioWorkers(t *testing.T, sc goldenScenario, workers int) goldenTrace {
	t.Helper()

	w, p := makeWorldWorkers(t, sc.gravity, workers)
	sc.setup(w)

	trace := goldenTrace{
		Scenario:  sc.name,
		TickCount: sc.tickCount,
		Frames:    []goldenFrame{},
		Events:    []goldenEvent{},
		Queries:   []goldenQuery{},
	}
	lastTick := sc.tickCount - 1

	// Single PostUpdate recorder: it runs after the physics pipeline (PreUpdate) so it observes
	// the post-step ECS writeback and this tick's flushed contact events.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn          spawnArchetype
		ContactBeginRx cardinal.WithSystemEventReceiver[physics.ContactBeginEvent]
		ContactEndRx   cardinal.WithSystemEventReceiver[physics.ContactEndEvent]
		TriggerBeginRx cardinal.WithSystemEventReceiver[physics.TriggerBeginEvent]
		TriggerEndRx   cardinal.WithSystemEventReceiver[physics.TriggerEndEvent]
	}) {
		tick := int(state.Tick())

		// Events: every tick. Kind order is fixed by this harness; order within a kind is the
		// backend's emission order.
		for e := range state.ContactBeginRx.Iter() {
			trace.Events = append(trace.Events, goldenEventOf(tick, "contact_begin", false, e.ContactEventPayload))
		}
		for e := range state.ContactEndRx.Iter() {
			trace.Events = append(trace.Events, goldenEventOf(tick, "contact_end", false, e.ContactEventPayload))
		}
		for e := range state.TriggerBeginRx.Iter() {
			trace.Events = append(trace.Events, goldenEventOf(tick, "trigger_begin", true, e.ContactEventPayload))
		}
		for e := range state.TriggerEndRx.Iter() {
			trace.Events = append(trace.Events, goldenEventOf(tick, "trigger_end", true, e.ContactEventPayload))
		}

		// Body state: reduced cadence plus the final tick.
		if tick%goldenSampleEvery == 0 || tick == lastTick {
			bodies := []goldenBody{}
			for eid, row := range state.Spawn.Iter() {
				tr := row.T.Get()
				vel := row.V.Get()
				bodies = append(bodies, goldenBody{
					Entity:   uint32(eid),
					Role:     row.Tag.Get().Role,
					PosX:     goldenFloat(tr.Position.X),
					PosY:     goldenFloat(tr.Position.Y),
					Rotation: goldenFloat(tr.Rotation),
					VelX:     goldenFloat(vel.Linear.X),
					VelY:     goldenFloat(vel.Linear.Y),
					AngVel:   goldenFloat(vel.Angular),
				})
			}
			slices.SortFunc(bodies, func(a, b goldenBody) int { return cmp.Compare(a.Entity, b.Entity) })
			trace.Frames = append(trace.Frames, goldenFrame{Tick: tick, Bodies: bodies})
		}

		// Queries: scenario-controlled cadence.
		if sc.queries != nil {
			for _, q := range sc.queries(p, tick) {
				q.Tick = tick
				if q.Hits == nil {
					q.Hits = []goldenQueryHit{}
				}
				trace.Queries = append(trace.Queries, q)
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, sc.tickCount)

	return trace
}

func goldenEventOf(tick int, kind string, sensor bool, p physics.ContactEventPayload) goldenEvent {
	return goldenEvent{
		Tick:        tick,
		Kind:        kind,
		Sensor:      sensor,
		EntityA:     uint32(p.EntityA),
		EntityB:     uint32(p.EntityB),
		ShapeIndexA: p.ShapeIndexA,
		ShapeIndexB: p.ShapeIndexB,
	}
}

// ---------------------------------------------------------------------------
// Query recording helpers
// ---------------------------------------------------------------------------

func goldenRaycast(name string, r physics.RaycastResult) goldenQuery {
	return goldenQuery{
		Name:       name,
		Hit:        r.Hit,
		Entity:     uint32(r.Entity),
		ShapeIndex: r.ShapeIndex,
		PointX:     goldenFloat(r.Point.X),
		PointY:     goldenFloat(r.Point.Y),
		NormalX:    goldenFloat(r.Normal.X),
		NormalY:    goldenFloat(r.Normal.Y),
		Fraction:   goldenFloat(r.Fraction),
		Hits:       []goldenQueryHit{},
	}
}

func goldenSweep(name string, r physics.CircleSweepResult) goldenQuery {
	return goldenQuery{
		Name:       name,
		Hit:        r.Hit,
		Entity:     uint32(r.Entity),
		ShapeIndex: r.ShapeIndex,
		PointX:     goldenFloat(r.Point.X),
		PointY:     goldenFloat(r.Point.Y),
		NormalX:    goldenFloat(r.Normal.X),
		NormalY:    goldenFloat(r.Normal.Y),
		Fraction:   goldenFloat(r.Fraction),
		Hits:       []goldenQueryHit{},
	}
}

// goldenOverlap records an AABB overlap result. Hits are sorted by (entity, shape index): the
// broadphase traversal order is a backend implementation detail and is not part of the contract.
func goldenOverlap(name string, r physics.AABBOverlapResult) goldenQuery {
	hits := make([]goldenQueryHit, 0, len(r.Hits))
	for _, h := range r.Hits {
		hits = append(hits, goldenQueryHit{Entity: uint32(h.Entity), ShapeIndex: h.ShapeIndex})
	}
	slices.SortFunc(hits, func(a, b goldenQueryHit) int {
		if c := cmp.Compare(a.Entity, b.Entity); c != 0 {
			return c
		}
		return cmp.Compare(a.ShapeIndex, b.ShapeIndex)
	})
	return goldenQuery{
		Name:     name,
		Hit:      len(hits) > 0,
		PointX:   goldenFloat(0),
		PointY:   goldenFloat(0),
		NormalX:  goldenFloat(0),
		NormalY:  goldenFloat(0),
		Fraction: goldenFloat(0),
		Hits:     hits,
	}
}

// ---------------------------------------------------------------------------
// Entity construction helper
// ---------------------------------------------------------------------------

// goldenSpawn registers an Init-hook system that creates the described entities once.
type goldenEntity struct {
	role     string
	pos      physics.Vec2
	rotation float64
	vel      physics.Velocity2D
	body     physics.PhysicsBody2D
}

func goldenSpawn(w *cardinal.World, entities func() []goldenEntity) {
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		for _, e := range entities() {
			_, row := state.Spawn.Create()
			row.Tag.Set(harnessTag{Role: e.role})
			row.T.Set(physics.Transform2D{Position: e.pos, Rotation: e.rotation})
			row.V.Set(e.vel)
			row.PB.Set(e.body)
		}
	}, cardinal.WithHook(cardinal.Init))
}

// ---------------------------------------------------------------------------
// Scenarios
// ---------------------------------------------------------------------------

func goldenScenarios() []goldenScenario {
	return []goldenScenario{
		goldenFallingCirclesFloor(),
		goldenBoxStack(),
		goldenCapsuleChainGround(),
		goldenSensorTrigger(),
		goldenFilterMatrix(),
		goldenQueriesOverTime(),
	}
}

// (a) Eight dynamic circles with varied radii and restitution dropped onto a static box floor.
func goldenFallingCirclesFloor() goldenScenario {
	return goldenScenario{
		name:      "falling_circles_floor",
		gravity:   physics.Vec2{X: 0, Y: -10},
		tickCount: 300,
		setup: func(w *cardinal.World) {
			goldenSpawn(w, func() []goldenEntity {
				out := []goldenEntity{{
					role: "floor",
					pos:  physics.Vec2{X: 0, Y: 0},
					body: newRigid(physics.BodyTypeStatic, physics.ColliderShape{
						ShapeType:    physics.ShapeTypeBox,
						HalfExtents:  physics.Vec2{X: 20, Y: 0.5},
						Friction:     0.4,
						CategoryBits: 0xFFFF,
						MaskBits:     0xFFFF,
					}),
				}}
				for i := range 8 {
					f := float64(i)
					out = append(out, goldenEntity{
						role: "circle_" + strconv.Itoa(i),
						pos:  physics.Vec2{X: -7 + 2*f, Y: 5 + 0.5*f},
						body: newRigid(physics.BodyTypeDynamic, physics.ColliderShape{
							ShapeType:    physics.ShapeTypeCircle,
							Radius:       0.2 + 0.05*f,
							Density:      1,
							Friction:     0.3,
							Restitution:  0.1 * f,
							CategoryBits: 0xFFFF,
							MaskBits:     0xFFFF,
						}),
					})
				}
				return out
			})
		},
	}
}

// (b) Five dynamic boxes stacked with a small lateral offset, settling onto a static floor.
func goldenBoxStack() goldenScenario {
	return goldenScenario{
		name:      "box_stack",
		gravity:   physics.Vec2{X: 0, Y: -10},
		tickCount: 300,
		setup: func(w *cardinal.World) {
			goldenSpawn(w, func() []goldenEntity {
				out := []goldenEntity{{
					role: "floor",
					pos:  physics.Vec2{X: 0, Y: 0},
					body: newRigid(physics.BodyTypeStatic, physics.ColliderShape{
						ShapeType:    physics.ShapeTypeBox,
						HalfExtents:  physics.Vec2{X: 10, Y: 0.5},
						Friction:     0.6,
						CategoryBits: 0xFFFF,
						MaskBits:     0xFFFF,
					}),
				}}
				for i := range 5 {
					f := float64(i)
					out = append(out, goldenEntity{
						role: "box_" + strconv.Itoa(i),
						// Small alternating X offset so the stack has to settle rather than
						// starting in a perfectly symmetric (and less interesting) pose.
						pos: physics.Vec2{X: 0.03 * f * float64(1-2*(i%2)), Y: 1.05 + 1.05*f},
						body: newRigid(physics.BodyTypeDynamic, physics.ColliderShape{
							ShapeType:    physics.ShapeTypeBox,
							HalfExtents:  physics.Vec2{X: 0.5, Y: 0.5},
							Density:      1,
							Friction:     0.6,
							Restitution:  0,
							CategoryBits: 0xFFFF,
							MaskBits:     0xFFFF,
						}),
					})
				}
				return out
			})
		},
	}
}

// (c) Four dynamic capsules and two dynamic convex polygons falling onto a static chain terrain.
func goldenCapsuleChainGround() goldenScenario {
	return goldenScenario{
		name:      "capsule_chain_ground",
		gravity:   physics.Vec2{X: 0, Y: -10},
		tickCount: 300,
		setup: func(w *cardinal.World) {
			goldenSpawn(w, func() []goldenEntity {
				out := []goldenEntity{{
					role: "terrain",
					pos:  physics.Vec2{X: 0, Y: 0},
					body: newRigid(physics.BodyTypeStatic, physics.ColliderShape{
						ShapeType: physics.ShapeTypeStaticChain,
						// Box2D v3 chains are one-sided: right-to-left (decreasing X) winding
						// gives upward-facing normals so bodies land on top.
						ChainPoints: []physics.Vec2{
							{X: 14, Y: 1}, {X: 7, Y: -1}, {X: 0, Y: -2},
							{X: -7, Y: -1}, {X: -14, Y: 1},
						},
						Friction:     0.5,
						CategoryBits: 0xFFFF,
						MaskBits:     0xFFFF,
					}),
				}}
				for i := range 4 {
					f := float64(i)
					out = append(out, goldenEntity{
						role: "capsule_" + strconv.Itoa(i),
						pos:  physics.Vec2{X: -6 + 4*f, Y: 6 + 0.7*f},
						body: newRigid(physics.BodyTypeDynamic, physics.ColliderShape{
							ShapeType:      physics.ShapeTypeCapsule,
							CapsuleCenter1: physics.Vec2{X: 0, Y: -0.4},
							CapsuleCenter2: physics.Vec2{X: 0, Y: 0.4},
							Radius:         0.25,
							Density:        1,
							Friction:       0.4,
							Restitution:    0.05,
							CategoryBits:   0xFFFF,
							MaskBits:       0xFFFF,
						}),
					})
				}
				for i := range 2 {
					f := float64(i)
					out = append(out, goldenEntity{
						role:     "poly_" + strconv.Itoa(i),
						pos:      physics.Vec2{X: -4 + 8*f, Y: 10 + f},
						rotation: 0.2 + 0.3*f,
						body: newRigid(physics.BodyTypeDynamic, physics.ColliderShape{
							ShapeType: physics.ShapeTypeConvexPolygon,
							Vertices: []physics.Vec2{
								{X: -0.5, Y: -0.4}, {X: 0.5, Y: -0.4}, {X: 0.35, Y: 0.5}, {X: -0.35, Y: 0.5},
							},
							Density:      1,
							Friction:     0.4,
							CategoryBits: 0xFFFF,
							MaskBits:     0xFFFF,
						}),
					})
				}
				return out
			})
		},
	}
}

// (d) A dynamic circle falls through a static sensor box, then lands on a solid floor. Captures
// trigger begin/end around the sensor plus contact begin on the floor.
func goldenSensorTrigger() goldenScenario {
	return goldenScenario{
		name:      "sensor_trigger",
		gravity:   physics.Vec2{X: 0, Y: -10},
		tickCount: 200,
		setup: func(w *cardinal.World) {
			goldenSpawn(w, func() []goldenEntity {
				return []goldenEntity{
					{
						role: "floor",
						pos:  physics.Vec2{X: 0, Y: 0},
						body: newRigid(physics.BodyTypeStatic, physics.ColliderShape{
							ShapeType:    physics.ShapeTypeBox,
							HalfExtents:  physics.Vec2{X: 8, Y: 0.5},
							Friction:     0.3,
							CategoryBits: 0xFFFF,
							MaskBits:     0xFFFF,
						}),
					},
					{
						role: "sensor_gate",
						pos:  physics.Vec2{X: 0, Y: 5},
						body: newRigid(physics.BodyTypeStatic, physics.ColliderShape{
							ShapeType:    physics.ShapeTypeBox,
							HalfExtents:  physics.Vec2{X: 2, Y: 0.5},
							IsSensor:     true,
							CategoryBits: 0xFFFF,
							MaskBits:     0xFFFF,
						}),
					},
					{
						role: "faller",
						pos:  physics.Vec2{X: 0, Y: 10},
						body: newRigid(physics.BodyTypeDynamic, physics.ColliderShape{
							ShapeType:    physics.ShapeTypeCircle,
							Radius:       0.3,
							Density:      1,
							Friction:     0.3,
							Restitution:  0.2,
							CategoryBits: 0xFFFF,
							MaskBits:     0xFFFF,
						}),
					},
				}
			})
		},
	}
}

// (e) Three category/mask/group combinations proving selective collision:
//   - lane A (X=-10): matching category/mask -> ball rests on its floor.
//   - lane B (X=0):   disjoint category/mask -> ball falls through its floor.
//   - lane C (X=10):  matching category/mask but a shared NEGATIVE group index, which in Box2D
//     overrides category/mask and forces "never collide" -> ball falls through.
func goldenFilterMatrix() goldenScenario {
	const (
		catA = 0x0001
		catB = 0x0002
		catC = 0x0004
		catX = 0x0008 // category nothing masks against
	)
	floor := func(role string, x float64, cat, mask uint64, group int32) goldenEntity {
		return goldenEntity{
			role: role,
			pos:  physics.Vec2{X: x, Y: 0},
			body: newRigid(physics.BodyTypeStatic, physics.ColliderShape{
				ShapeType:    physics.ShapeTypeBox,
				HalfExtents:  physics.Vec2{X: 3, Y: 0.5},
				Friction:     0.3,
				CategoryBits: cat,
				MaskBits:     mask,
				GroupIndex:   group,
			}),
		}
	}
	ball := func(role string, x float64, cat, mask uint64, group int32) goldenEntity {
		return goldenEntity{
			role: role,
			pos:  physics.Vec2{X: x, Y: 5},
			body: newRigid(physics.BodyTypeDynamic, physics.ColliderShape{
				ShapeType:    physics.ShapeTypeCircle,
				Radius:       0.4,
				Density:      1,
				Friction:     0.3,
				CategoryBits: cat,
				MaskBits:     mask,
				GroupIndex:   group,
			}),
		}
	}
	return goldenScenario{
		name:      "filter_matrix",
		gravity:   physics.Vec2{X: 0, Y: -10},
		tickCount: 200,
		setup: func(w *cardinal.World) {
			goldenSpawn(w, func() []goldenEntity {
				return []goldenEntity{
					floor("floor_match", -10, catA, catA, 0),
					ball("ball_match", -10, catA, catA, 0),
					floor("floor_disjoint", 0, catB, catB, 0),
					ball("ball_disjoint", 0, catX, catX, 0),
					floor("floor_group", 10, catC, catC, -7),
					ball("ball_group", 10, catC, catC, -7),
				}
			})
		},
	}
}

// (f) Static geometry plus a kinematic mover and a drifting dynamic ball, with a raycast, an AABB
// overlap and a circle sweep executed every 10 ticks. Zero gravity keeps the motion analytic so
// the query results sweep across the scene predictably.
func goldenQueriesOverTime() goldenScenario {
	return goldenScenario{
		name:      "queries_over_time",
		gravity:   physics.Vec2{X: 0, Y: 0},
		tickCount: 150,
		setup: func(w *cardinal.World) {
			goldenSpawn(w, func() []goldenEntity {
				return []goldenEntity{
					{
						role: "wall",
						pos:  physics.Vec2{X: 5, Y: 0},
						body: newRigid(physics.BodyTypeStatic, physics.ColliderShape{
							ShapeType:    physics.ShapeTypeBox,
							HalfExtents:  physics.Vec2{X: 0.5, Y: 2},
							CategoryBits: 0xFFFF,
							MaskBits:     0xFFFF,
						}),
					},
					{
						role: "pillar",
						pos:  physics.Vec2{X: -6, Y: 0},
						body: newRigid(physics.BodyTypeStatic, physics.ColliderShape{
							ShapeType:    physics.ShapeTypeCircle,
							Radius:       0.75,
							CategoryBits: 0xFFFF,
							MaskBits:     0xFFFF,
						}),
					},
					{
						role: "mover",
						pos:  physics.Vec2{X: -8, Y: 3},
						vel:  physics.Velocity2D{Linear: physics.Vec2{X: 2, Y: 0}},
						body: newRigidNoGravity(physics.BodyTypeKinematic, physics.ColliderShape{
							ShapeType:    physics.ShapeTypeBox,
							HalfExtents:  physics.Vec2{X: 0.5, Y: 0.5},
							CategoryBits: 0xFFFF,
							MaskBits:     0xFFFF,
						}),
					},
					{
						role: "drifter",
						pos:  physics.Vec2{X: -8, Y: -3},
						vel:  physics.Velocity2D{Linear: physics.Vec2{X: 1.5, Y: 0}, Angular: 0.5},
						body: newRigidNoGravity(physics.BodyTypeDynamic, physics.ColliderShape{
							ShapeType:    physics.ShapeTypeCircle,
							Radius:       0.35,
							Density:      1,
							CategoryBits: 0xFFFF,
							MaskBits:     0xFFFF,
						}),
					},
				}
			})
		},
		queries: func(p *physics.Plugin, tick int) []goldenQuery {
			if tick%goldenSampleEvery != 0 {
				return nil
			}
			// Ray along the drifter's lane: only the drifter sits at Y=-3, so the hit fraction
			// tracks its motion instead of being pinned to static geometry.
			ray := p.Raycast(physics.RaycastRequest{
				Origin: physics.Vec2{X: -12, Y: -3},
				End:    physics.Vec2{X: 12, Y: -3},
			})
			// Box narrow enough that the mover and then the drifter leave it mid-run, so the hit
			// set shrinks over time.
			ov := p.OverlapAABB(physics.AABBOverlapRequest{
				Min: physics.Vec2{X: -9, Y: -4},
				Max: physics.Vec2{X: -5, Y: 4},
			})
			sweep := p.CircleSweep(physics.CircleSweepRequest{
				Start:  physics.Vec2{X: -12, Y: 3},
				End:    physics.Vec2{X: 12, Y: 3},
				Radius: 0.4,
			})
			return []goldenQuery{
				goldenRaycast("ray_horizontal", ray),
				goldenOverlap("overlap_left_box", ov),
				goldenSweep("sweep_mover_lane", sweep),
			}
		},
	}
}

// ---------------------------------------------------------------------------
// Test entry point
// ---------------------------------------------------------------------------

// TestGoldenTrace verifies (default) or re-records (generator mode) the reference behavior. The
// committed fixtures were captured from the now-removed CGO Box2D backend; see the file header
// before using generator mode. Verify mode runs in parallel — the pure-Go backend is
// per-plugin-instance state. Generator mode stays serial while rewriting fixtures.
func TestGoldenTrace(t *testing.T) {
	update := os.Getenv(goldenUpdateEnv) == "1"
	if !update {
		t.Parallel()
	}
	for _, sc := range goldenScenarios() {
		t.Run(sc.name, func(t *testing.T) {
			if !update {
				t.Parallel()
			}
			got := runGoldenScenario(t, sc)
			path := filepath.Join("testdata", "golden_"+sc.name+".json")

			if update {
				require.NoError(t, os.MkdirAll("testdata", 0o750))
				blob, err := json.MarshalIndent(got, "", "  ")
				require.NoError(t, err)
				blob = append(blob, '\n')
				require.NoError(t, os.WriteFile(path, blob, 0o600))
				t.Logf("wrote %s: %d bytes, %d frames, %d events, %d queries",
					path, len(blob), len(got.Frames), len(got.Events), len(got.Queries))
				return
			}

			raw, err := os.ReadFile(path)
			require.NoErrorf(t, err, "missing golden fixture %s; regenerate with %s=1", path, goldenUpdateEnv)
			var want goldenTrace
			require.NoError(t, json.Unmarshal(raw, &want))
			compareGoldenTrace(t, want, got)
		})
	}
}

// compareGoldenTrace requires identical structure (entity sets, event sequences, query hit
// identity) and numeric agreement within goldenDelta.
func compareGoldenTrace(t *testing.T, want, got goldenTrace) {
	t.Helper()

	require.Equal(t, want.Scenario, got.Scenario, "scenario name")
	require.Equal(t, want.TickCount, got.TickCount, "tick count")

	// Events must match EXACTLY, including sequence.
	require.Equal(t, want.Events, got.Events, "contact/trigger event sequence")

	require.Len(t, got.Frames, len(want.Frames), "number of recorded frames")
	for i := range want.Frames {
		wf, gf := want.Frames[i], got.Frames[i]
		require.Equalf(t, wf.Tick, gf.Tick, "frame %d tick", i)
		require.Lenf(t, gf.Bodies, len(wf.Bodies), "frame tick %d body count", wf.Tick)
		for j := range wf.Bodies {
			wb, gb := wf.Bodies[j], gf.Bodies[j]
			where := "tick " + strconv.Itoa(wf.Tick) + " body " + strconv.Itoa(int(wb.Entity)) + " (" + wb.Role + ")"
			require.Equalf(t, wb.Entity, gb.Entity, "%s: entity id", where)
			require.Equalf(t, wb.Role, gb.Role, "%s: role", where)
			goldenInDelta(t, wb.PosX, gb.PosX, where+" pos_x")
			goldenInDelta(t, wb.PosY, gb.PosY, where+" pos_y")
			goldenInDelta(t, wb.Rotation, gb.Rotation, where+" rotation")
			goldenInDelta(t, wb.VelX, gb.VelX, where+" vel_x")
			goldenInDelta(t, wb.VelY, gb.VelY, where+" vel_y")
			goldenInDelta(t, wb.AngVel, gb.AngVel, where+" ang_vel")
		}
	}

	require.Len(t, got.Queries, len(want.Queries), "number of recorded queries")
	for i := range want.Queries {
		wq, gq := want.Queries[i], got.Queries[i]
		where := "query " + wq.Name + " at tick " + strconv.Itoa(wq.Tick)
		require.Equalf(t, wq.Tick, gq.Tick, "%s: tick", where)
		require.Equalf(t, wq.Name, gq.Name, "%s: name", where)
		require.Equalf(t, wq.Hit, gq.Hit, "%s: hit flag", where)
		require.Equalf(t, wq.Entity, gq.Entity, "%s: entity", where)
		require.Equalf(t, wq.ShapeIndex, gq.ShapeIndex, "%s: shape index", where)
		require.Equalf(t, wq.Hits, gq.Hits, "%s: overlap hits", where)
		goldenInDelta(t, wq.PointX, gq.PointX, where+" point_x")
		goldenInDelta(t, wq.PointY, gq.PointY, where+" point_y")
		goldenInDelta(t, wq.NormalX, gq.NormalX, where+" normal_x")
		goldenInDelta(t, wq.NormalY, gq.NormalY, where+" normal_y")
		goldenInDelta(t, wq.Fraction, gq.Fraction, where+" fraction")
	}
}

func goldenInDelta(t *testing.T, want, got, where string) {
	t.Helper()
	if want == got {
		return // exact bit-for-bit match, the common case
	}
	require.InDelta(t, goldenParse(t, want, where), goldenParse(t, got, where), goldenDelta, where)
}
