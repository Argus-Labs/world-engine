package harness

import (
	"fmt"
	"hash/fnv"
	"math"
	"reflect"
	"sort"
	"testing"
	"time"
	"unsafe"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// TickRate is the simulation rate for both Cardinal and the physics plugin.
// The two must match or simulated time drifts from tick time.
const TickRate = 60.0

// TailTicks is how many extra ticks run after the last scheduled step, so a
// scenario's final mutation still gets simulated and the NaN watchdog still sees
// the resulting state.
const TailTicks = 10

// eventStore records physics events per scenario and remembers which scenario
// and label each entity belonged to. Entities are never removed from the owner
// map: end events can arrive for an entity that has already been destroyed.
type eventStore struct {
	owners map[cardinal.EntityID]owner
	events map[string][]LoggedEvent
}

type owner struct {
	scenario string
	label    string
}

func newEventStore() *eventStore {
	return &eventStore{
		owners: map[cardinal.EntityID]owner{},
		events: map[string][]LoggedEvent{},
	}
}

func (s *eventStore) own(id cardinal.EntityID, scenario, label string) {
	s.owners[id] = owner{scenario: scenario, label: label}
}

func (s *eventStore) label(id cardinal.EntityID) string {
	if o, ok := s.owners[id]; ok {
		return o.label
	}
	return fmt.Sprintf("entity#%d", id)
}

func (s *eventStore) record(kind EventKind, tick uint64, p physics.ContactEventPayload) {
	o, ok := s.owners[p.EntityA]
	if !ok {
		o, ok = s.owners[p.EntityB]
	}
	if !ok {
		return // Not a harness body; nothing owns it.
	}
	s.events[o.scenario] = append(s.events[o.scenario], LoggedEvent{Kind: kind, Tick: tick, Payload: p})
}

func (s *eventStore) forScenario(scenario string, kind EventKind) []LoggedEvent {
	all := s.events[scenario]
	out := make([]LoggedEvent, 0, len(all))
	for _, e := range all {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

// Config controls how the harness builds its world.
type Config struct {
	// Gravity is the world gravity vector handed to the physics plugin.
	Gravity physics.Vec2
	// SubStepCount is the plugin's solver sub-step count; zero uses the plugin default.
	SubStepCount int
	// Verbose prints passing checks as they happen, not just failures.
	Verbose bool
	// ExtraTicks runs the loop longer than the scenarios strictly require.
	ExtraTicks uint64
	// PreCapture, when set, receives a copy of every body's components at the
	// start of each tick, before the physics plugin runs.
	PreCapture *Capture
	// PostCapture, when set, receives the same after the physics pipeline has
	// reconciled, stepped and written back.
	PostCapture *Capture
	// Workers is the physics engine's worker count. Results are identical for
	// every value, so this only changes throughput.
	Workers int
	// Digest prints a digest of every body's final state at the end of the run.
	// Two runs of the same binary must produce the same digest; if they do not,
	// something in the pipeline is reading wall-clock time, map order, or
	// uninitialised memory.
	Digest bool
	// TB, when set, routes every check to a testing.TB as well as the report:
	// failures through Errorf at the scenario's call site, notes and skips through
	// Logf. The go test entry points set it; the CLI leaves it nil.
	TB testing.TB
}

// Runner owns the scenario list, the report, and the tick loop.
type Runner struct {
	report      *Report
	events      *eventStore
	nanReported map[cardinal.EntityID]bool
	scenarios   []*Scenario
	lastTick    uint64
	plugin      *physics.Plugin
	worldSeen   bool
	resetOK     bool
	digest      bool
	ticked      int
}

// New builds a runner over the given scenarios, assigning each one its own lane.
func New(scenarios []Scenario, cfg Config) *Runner {
	r := &Runner{
		report:      NewReport(cfg.Verbose),
		events:      newEventStore(),
		nanReported: map[cardinal.EntityID]bool{},
	}
	for i := range scenarios {
		s := scenarios[i]
		s.lane = float64(i) * LaneWidth
		r.scenarios = append(r.scenarios, &s)
		if last := s.LastTick(); last > r.lastTick {
			r.lastTick = last
		}
	}
	r.lastTick += TailTicks + cfg.ExtraTicks
	r.digest = cfg.Digest
	if cfg.TB != nil {
		r.report.BindTB(cfg.TB)
	}
	return r
}

// allowWorldReset tells the world watchdog that the next disappearance of the
// C-side world is deliberate. physics2d.ResetRuntime is global, so the scenario
// that calls it has to say so or every other lane reports a dead world.
func (r *Runner) allowWorldReset() { r.resetOK = true }

// Report returns the accumulated results.
func (r *Runner) Report() *Report { return r.report }

// Plugin returns the physics plugin bound to this runner's world. It is nil
// until BuildWorld has run.
func (r *Runner) Plugin() *physics.Plugin { return r.plugin }

// LastTick returns the final tick the loop will run.
func (r *Runner) LastTick() uint64 { return r.lastTick }

func (r *Runner) ctx(scenario *Scenario, probes *Probes, tick uint64) *Ctx {
	return &Ctx{
		report:     r.report,
		probes:     probes,
		events:     r.events,
		plugin:     r.plugin,
		scenario:   scenario.Name,
		lane:       scenario.lane,
		tick:       tick,
		allowReset: r.allowWorldReset,
		tb:         r.report.tb,
	}
}

// -----------------------------------------------------------------------------
// Systems
// -----------------------------------------------------------------------------

// setupState runs every scenario's Setup on cardinal.Init. It is registered
// before the plugin so the plugin's InitPhysicsSystem sees all the bodies during
// its first FullRebuildFromECS.
type setupState struct {
	cardinal.BaseSystemState
	Probes Probes
}

// preStepState runs every scenario's EachTick on PreUpdate. It is registered
// before the plugin so gameplay writes land in ECS before the reconciler reads
// them in the same tick.
type preStepState struct {
	cardinal.BaseSystemState
	Probes Probes
}

// stepState runs scheduled steps on Update, after the physics pipeline has
// reconciled, stepped and written back, and while this tick's contact events are
// still readable.
type stepState struct {
	cardinal.BaseSystemState
	Probes       Probes
	ContactBegin cardinal.WithSystemEventReceiver[physics.ContactBeginEvent]
	ContactEnd   cardinal.WithSystemEventReceiver[physics.ContactEndEvent]
	TriggerBegin cardinal.WithSystemEventReceiver[physics.TriggerBeginEvent]
	TriggerEnd   cardinal.WithSystemEventReceiver[physics.TriggerEndEvent]
}

func (r *Runner) setup(state *setupState) {
	for _, s := range r.scenarios {
		if s.Setup == nil {
			continue
		}
		s.Setup(r.ctx(s, &state.Probes, 0))
	}
}

func (r *Runner) preStep(state *preStepState) {
	tick := state.Tick()
	for _, s := range r.scenarios {
		if s.EachTick == nil {
			continue
		}
		s.EachTick(r.ctx(s, &state.Probes, tick))
	}
}

func (r *Runner) step(state *stepState) {
	tick := state.Tick()

	for e := range state.ContactBegin.Iter() {
		r.events.record(ContactBegin, tick, e.ContactEventPayload)
	}
	for e := range state.ContactEnd.Iter() {
		r.events.record(ContactEnd, tick, e.ContactEventPayload)
	}
	for e := range state.TriggerBegin.Iter() {
		r.events.record(TriggerBegin, tick, e.ContactEventPayload)
	}
	for e := range state.TriggerEnd.Iter() {
		r.events.record(TriggerEnd, tick, e.ContactEventPayload)
	}

	r.watchNaN(state, tick)
	r.watchWorld(tick)

	for _, s := range r.scenarios {
		for i := range s.Steps {
			if s.Steps[i].Tick != tick || s.Steps[i].Do == nil {
				continue
			}
			s.Steps[i].Do(r.ctx(s, &state.Probes, tick))
		}
	}
}

// watchNaN fails once per entity the first time any of its physics scalars stops
// being finite. A NaN anywhere in the pipeline poisons the whole Box2D island,
// so catching the first one names the body actually at fault.
func (r *Runner) watchNaN(state *stepState, tick uint64) {
	for eid, row := range state.Probes.Iter() {
		if r.nanReported[eid] {
			continue
		}
		t := row.Transform.Get()
		v := row.Velocity.Get()
		bad := ""
		switch {
		case !finite(t.Position.X) || !finite(t.Position.Y):
			bad = fmt.Sprintf("position=(%v, %v)", t.Position.X, t.Position.Y)
		case !finite(t.Rotation):
			bad = fmt.Sprintf("rotation=%v", t.Rotation)
		case !finite(v.Linear.X) || !finite(v.Linear.Y):
			bad = fmt.Sprintf("velocity=(%v, %v)", v.Linear.X, v.Linear.Y)
		case !finite(v.Angular):
			bad = fmt.Sprintf("angular=%v", v.Angular)
		}
		if bad == "" {
			continue
		}
		r.nanReported[eid] = true
		p := row.Probe.Get()
		r.report.Fail(p.Scenario, "no NaN/Inf in simulated state", tick,
			"body %q (entity %d) went non-finite: %s", p.Label, eid, bad)
	}
}

// watchWorld fails if the Box2D world disappears mid-run without a
// scenario having deliberately reset it. WorldID is 0 before the first reconcile
// and after Reset. Engine() is nil before the first reconcile and after Reset, so only a
// transition from live back to nil is a bug. The permission a scenario grants is
// consumed on the next tick either way,
// so it cannot leave the watchdog switched off for the rest of the run.
func (r *Runner) watchWorld(tick uint64) {
	allowed := r.resetOK
	r.resetOK = false

	if r.plugin != nil && r.plugin.Engine() != nil {
		r.worldSeen = true
		return
	}
	if r.worldSeen && !allowed {
		r.report.Fail("runtime", "the Box2D world stays alive", tick,
			"Plugin.Engine() went nil after a world had been created")
	}
	r.worldSeen = false
}

// -----------------------------------------------------------------------------
// World construction and the tick loop
// -----------------------------------------------------------------------------

// BuildWorld creates the Cardinal world and registers everything in the order the
// physics pipeline requires: scenario Init and PreUpdate systems first, then the
// plugin, then the Update-hook step system.
func (r *Runner) BuildWorld(cfg Config) (*cardinal.World, error) {
	debug := false
	world, err := cardinal.NewWorld(cardinal.WorldOptions{
		Region:              "local",
		Organization:        "physics-test",
		Project:             "physics-test",
		ShardID:             "0",
		TickRate:            TickRate,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        math.MaxUint32,
		Debug:               &debug,
	})
	if err != nil {
		return nil, err
	}

	cardinal.RegisterSystem(world, r.setup, cardinal.WithHook(cardinal.Init))
	cardinal.RegisterSystem(world, r.preStep, cardinal.WithHook(cardinal.PreUpdate))

	// Registered ahead of the plugin so it sees ECS as the tick found it. After
	// a snapshot restore that is the deserialized state, before anything has had
	// a chance to overwrite it.
	if cfg.PreCapture != nil {
		RegisterPreCapture(world, cfg.PreCapture)
	}

	// The plugin instance owns this world's physics runtime — there is no
	// package-level state any more — so the harness has to keep it: queries,
	// Reset and Engine are all methods on it, and registering one instance with
	// two worlds panics.
	r.plugin = physics.NewPlugin(physics.Config{
		Gravity:      cfg.Gravity,
		TickRate:     TickRate,
		SubStepCount: cfg.SubStepCount,
		Workers:      cfg.Workers,
	})
	cardinal.RegisterPlugin(world, r.plugin)

	cardinal.RegisterSystem(world, r.step, cardinal.WithHook(cardinal.Update))

	if cfg.PostCapture != nil {
		RegisterPostCapture(world, cfg.PostCapture)
	}
	return world, nil
}

// Advance ticks the world n times, continuing from wherever it left off. The
// timestamps are derived from the tick index so two worlds ticked the same
// number of times see the same sequence.
func (r *Runner) Advance(world *cardinal.World, n int) {
	for range n {
		world.Tick(time.Unix(int64(r.ticked), 0))
		r.ticked++
	}
}

// Ticked reports how many ticks Advance has run.
func (r *Runner) Ticked() int { return r.ticked }

// EventsUpTo counts events of a kind recorded for a scenario on or before a tick.
// The restore check uses it to tell "the rebuilt world replayed every existing
// overlap as new" apart from "the rebuilt world kept its dedupe baseline".
func (r *Runner) EventsUpTo(scenario string, kind EventKind, tick uint64) int {
	n := 0
	for _, e := range r.events.forScenario(scenario, kind) {
		if e.Tick <= tick {
			n++
		}
	}
	return n
}

// Run initializes the world, ticks it to completion, prints the report, and
// returns the process exit code (0 when every check passed).
func (r *Runner) Run(world *cardinal.World) int {
	InitECS(world)

	// Bound to a testing.TB, results already reach the test log line by line;
	// the banner and the summary table are for the CLI.
	if !r.report.Bound() {
		fmt.Printf("running %d scenario(s) for %d ticks at %.0f Hz\n\n",
			len(r.scenarios), r.lastTick+1, TickRate)
	}

	// Deterministic timestamps: the physics step uses a fixed dt, so wall clock
	// must not leak into the simulation.
	r.Advance(world, int(r.lastTick)+1) //nolint:gosec // tick counts are small; G115 flags the uint64->int conversion

	if r.digest {
		r.printDigest(world)
	}

	if !r.report.Bound() {
		r.report.Print()
	}
	if _, fail, _ := r.report.Totals(); fail > 0 {
		return 1
	}
	return 0
}

// digestState is one body's final state, keyed by a stable label rather than by
// entity ID so the digest does not move when a scenario adds a body earlier in
// the run.
type digestState struct {
	key                     string
	px, py, rot, vx, vy, av float64
}

// printDigest reports a hash over every surviving body's final pose and
// velocity, plus the extremes, so two runs can be compared at a glance.
// Digest hashes every probe body's final pose and velocity and returns the body
// count with the hash. Two runs of the same build on the same machine must agree,
// and so must runs that differ only in Config.Workers.
func (r *Runner) Digest(world *cardinal.World) (int, uint64) {
	var rows []digestState
	collect := func(state *digestCollectorState) {
		for eid, row := range state.Probes.Iter() {
			p := row.Probe.Get()
			t := row.Transform.Get()
			v := row.Velocity.Get()
			rows = append(rows, digestState{
				key: fmt.Sprintf("%s/%s/%d", p.Scenario, p.Label, eid),
				px:  t.Position.X, py: t.Position.Y, rot: t.Rotation,
				vx: v.Linear.X, vy: v.Linear.Y, av: v.Angular,
			})
		}
	}
	runOnce(world, collect)

	sort.Slice(rows, func(i, j int) bool { return rows[i].key < rows[j].key })

	h := fnv.New64a()
	for _, row := range rows {
		fmt.Fprintf(h, "%s|%v|%v|%v|%v|%v|%v\n",
			row.key, row.px, row.py, row.rot, row.vx, row.vy, row.av)
	}
	return len(rows), h.Sum64()
}

func (r *Runner) printDigest(world *cardinal.World) {
	n, h := r.Digest(world)
	fmt.Printf("\ndigest: bodies=%d fnv1a64=%016x\n", n, h)
	fmt.Println("(run the binary twice and compare; the same build on the same " +
		"machine must produce the same digest)")
}

// digestCollectorState is a throwaway system state used only to walk every body
// once at the end of the run.
type digestCollectorState struct {
	cardinal.BaseSystemState
	Probes Probes
}

// runOnce registers fn as a one-shot Update system and ticks the world once so
// it can read component state through a properly initialised search.
func runOnce(world *cardinal.World, fn func(*digestCollectorState)) {
	done := false
	cardinal.RegisterSystem(world, func(state *digestCollectorState) {
		if done {
			return
		}
		done = true
		fn(state)
	}, cardinal.WithHook(cardinal.Update))
	world.Tick(time.Unix(1<<20, 0))
}

// InitECS runs the world's Init-hook systems and marks the ECS world initialized.
//
// cardinal.World only does this inside StartGame, which also stands up NATS and
// the ConnectRPC service. This test game drives World.Tick directly so it can run
// with no infrastructure at all, so it reaches the unexported ecs.World through
// reflection — the same approach the plugin's own integration tests use.
func InitECS(world *cardinal.World) {
	v := reflect.ValueOf(world).Elem()
	f := v.FieldByName("world")
	if !f.IsValid() {
		panic("cardinal.World: no 'world' field; the headless init shim needs updating")
	}
	inner := reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	m := inner.MethodByName("Init")
	if !m.IsValid() {
		panic("ecs.World: no Init method; the headless init shim needs updating")
	}
	m.Call(nil)
}
