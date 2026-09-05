// Package restore drives the crash-restore check: snapshot a running world,
// rebuild it in a fresh one, and compare.
//
// This is the path a shard takes after a crash, and it is not the same path as
// physics2d.ResetRuntime on a live world. A restore round-trips every component
// through MarshalWire and UnmarshalWire, which for PhysicsBody2D means going
// through its custom UnmarshalJSON — the code that decides whether an absent
// field means "false" or "Box2D's default of true". Only a real snapshot
// exercises that.
//
// Cardinal's own ordering is reproduced exactly: World.run calls world.Init()
// first, so the game's Init systems spawn a whole scene and the physics plugin
// builds a Box2D world from it, and only *then* does restore() call FromProto
// and throw that ECS state away. The C-side world is left describing a scene
// that no longer exists, which is why the plugin documents calling ResetRuntime
// after FromProto. Both variants are available here.
package restore

import (
	"fmt"
	"os"
	"strings"

	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/scenario"

	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

const (
	// settleTicks runs long enough for the stack to come to rest, bodies to fall
	// asleep, and contacts and triggers to be established before the snapshot.
	settleTicks = 240
	// driftTicks is how far both worlds run after the restore. Long enough that
	// a lost flag moves a body metres, short enough that solver noise stays in
	// the millimetres.
	driftTicks = 120

	// exactTol covers float64 JSON round-tripping, which should be lossless.
	exactTol = 1e-12
	// driftTol is what a rebuilt world may differ by after driftTicks. Warm-start
	// impulses and sleep timers do not survive a rebuild, so bodies in active
	// contact settle a hair differently; anything larger is a lost field.
	driftTol = 0.25
)

// Run performs the check and returns the process exit code.
func Run(cfg harness.Config, callResetRuntime bool) int {
	report := harness.NewReport(cfg.Verbose)
	if cfg.TB != nil {
		report.BindTB(cfg.TB)
	}
	const check = "restore"

	fmt.Printf("crash-restore check: settle %d ticks, snapshot, restore, "+
		"run both worlds %d more ticks\n", settleTicks, driftTicks)
	if !callResetRuntime {
		fmt.Println("running WITHOUT the documented Plugin.Reset after FromProto, " +
			"to see whether the reconciler heals the stale Box2D world on its own")
	}
	fmt.Println()

	// --- The original world.
	//
	// The capturer swaps in a freshly allocated map every tick rather than
	// mutating the old one, so copying the Capture struct freezes that tick's
	// state and later ticks cannot reach back into it.
	var live harness.Capture
	originalCfg := cfg
	originalCfg.PostCapture = &live

	original := harness.New([]harness.Scenario{scenario.RestoreScene()}, originalCfg)
	worldA, err := original.BuildWorld(originalCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build the original world: %v\n", err)
		return 2
	}
	harness.InitECS(worldA)
	original.Advance(worldA, settleTicks)

	snapshot, err := harness.SnapshotWorld(worldA)
	if err != nil {
		report.Fail(check, "the world serializes to a snapshot", uint64(settleTicks),
			"ToProto failed: %v", err)
		if !report.Bound() {
			report.Print()
		}
		return 1
	}
	report.Pass(check, "the world serializes to a snapshot", uint64(settleTicks))

	// Freeze what the snapshot describes, then let the original keep running so
	// there is something to compare the restored world's future against.
	atSnapshot := live
	original.Advance(worldA, driftTicks)
	atEnd := live

	// --- The replacement world, built exactly the way a restarted shard is.
	var restoredAtStart, restoredAtEnd harness.Capture
	replacementCfg := cfg
	replacementCfg.PreCapture = &restoredAtStart
	replacementCfg.PostCapture = &restoredAtEnd

	replacement := harness.New([]harness.Scenario{scenario.RestoreScene()}, replacementCfg)
	worldB, err := replacement.BuildWorld(replacementCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build the replacement world: %v\n", err)
		return 2
	}

	// Init first, restore second — Cardinal's order, not a convenient one.
	harness.InitECS(worldB)
	if err := harness.RestoreWorld(worldB, snapshot); err != nil {
		report.Fail(check, "the snapshot deserializes into a fresh world", 0,
			"FromProto failed: %v", err)
		if !report.Bound() {
			report.Print()
		}
		return 1
	}
	report.Pass(check, "the snapshot deserializes into a fresh world", 0)

	if callResetRuntime {
		replacement.Plugin().Reset()
	}

	// One tick: the pre-plugin capture sees the deserialized ECS untouched, and
	// the physics pipeline then brings Box2D in line with it.
	replacement.Advance(worldB, 1)

	checkAwakeIsNotStale(report, check, atSnapshot)
	checkDeserialized(report, check, atSnapshot, restoredAtStart)
	checkRebuild(report, check, replacement.Plugin())

	// Match the original's step count. After Plugin.Reset that first tick found
	// no C-side world, so it rebuilt and returned without stepping and the
	// replacement is one step behind. Without Plugin.Reset the world was still
	// there, so the tick stepped like any other and the replacement is level.
	remaining := driftTicks
	if !callResetRuntime {
		remaining--
	}
	replacement.Advance(worldB, remaining)
	checkDrift(report, check, atEnd, restoredAtEnd)

	if !callResetRuntime {
		report.Note(check, uint64(settleTicks+driftTicks),
			"this run skipped Plugin.Reset, so the reconciler diffed the restored "+
				"components against the shadow left by this world's own Init scene "+
				"and only pushed what changed. That is why fewer stale fields are "+
				"re-applied here — not because skipping Plugin.Reset is safe. It "+
				"works only while the Init scene happens to match the snapshot "+
				"entity for entity, which a real restore cannot rely on")
	}

	if !report.Bound() {
		report.Print()
	}
	if _, fail, _ := report.Totals(); fail > 0 {
		return 1
	}
	return 0
}

// checkAwakeIsNotStale looks for bodies whose PhysicsBody2D.Awake contradicts
// what they are actually doing.
//
// Transform2D and Velocity2D are mirrored back from Box2D every tick, so they are
// always current. Awake is not: the reconciler pushes it into Box2D and writeback
// never reads Box2D's real sleep state back. A body spawned with Awake=false and
// later woken by a collision therefore keeps Awake=false in the component
// forever, and every rebuild recreates it asleep.
//
// This runs on the snapshot state, before any rebuild, because the stale field is
// the hazard whether or not a particular run happens to expose it.
func checkAwakeIsNotStale(report *harness.Report, check string, at harness.Capture) {
	const movingEnough = 0.5
	var stale []string
	for _, label := range at.Labels() {
		row := at.Rows[label]
		if row.Body.Awake || !row.Body.Active {
			continue
		}
		speed := abs(row.Velocity.Linear.X) + abs(row.Velocity.Linear.Y)
		if speed > movingEnough {
			stale = append(stale, fmt.Sprintf("%s (Awake=false, moving at %.2f m/s)",
				label, speed))
		}
	}
	if len(stale) == 0 {
		report.Pass(check, "no body's Awake flag contradicts its motion", uint64(settleTicks))
		return
	}
	report.Fail(check, "no body's Awake flag contradicts its motion", uint64(settleTicks),
		"%d body(s) are moving while their component says Awake=false, so a rebuild "+
			"will recreate them asleep and frozen in mid-air:\n       %s\n       "+
			"Transform2D and Velocity2D are written back from Box2D every tick; "+
			"Awake is push-only, so it goes stale the moment physics wakes a body "+
			"the game had put to sleep",
		len(stale), strings.Join(stale, "\n       "))
}

// checkDeserialized compares ECS as the restored world found it against ECS as
// the snapshot left it. Nothing has simulated yet, so this is purely a test of
// MarshalWire, UnmarshalWire and PhysicsBody2D.UnmarshalJSON.
func checkDeserialized(report *harness.Report, check string, want, got harness.Capture) {
	if !expectSameBodies(report, check+" (deserialize)", want, got, 0) {
		return
	}

	diffs := harness.CompareCaptures(want, got, exactTol)
	if len(diffs) == 0 {
		report.Pass(check, "every component survives the snapshot round trip byte for byte", 0)
		report.Note(check, 0, "%d bodies round-tripped through MarshalWire/UnmarshalWire "+
			"with no field changed", len(want.Rows))
		return
	}
	report.Fail(check, "every component survives the snapshot round trip byte for byte", 0,
		"%d field(s) changed across the snapshot:\n       %s",
		len(diffs), joinDiffs(diffs))
}

// checkRebuild confirms the restored ECS actually reached Box2D. Reaching this
// function at all is half the check: a rebuild that trips a Box2D assertion
// terminates the process instead of returning.
func checkRebuild(report *harness.Report, check string, plugin *physics.Plugin) {
	report.Pass(check, "the world survives the tick that rebuilds it", 1)
	if plugin.Engine() == nil {
		report.Fail(check, "the restored world rebuilds its Box2D world", 1,
			"Plugin.Engine() is still nil after a full tick; nothing was rebuilt")
		return
	}
	report.Pass(check, "the restored world rebuilds its Box2D world", 1)
}

// checkDrift compares the two worlds after both have simulated the same number
// of steps from the same state. Exact equality is not expected: a rebuild
// discards warm-start impulses and sleep timers, so bodies in contact settle
// slightly differently. A field lost in the restore moves a body much further
// than that.
func checkDrift(report *harness.Report, check string, want, got harness.Capture) {
	tick := uint64(settleTicks + driftTicks)
	if !expectSameBodies(report, check+" (rebuild)", want, got, tick) {
		return
	}

	var worst float64
	var worstDiff harness.Diff
	var offenders []harness.Diff
	for _, diff := range harness.CompareCaptures(want, got, driftTol) {
		if !strings.HasPrefix(diff.Field, "Transform.Position") {
			continue
		}
		offenders = append(offenders, diff)
		var g, w float64
		if _, err := fmt.Sscan(diff.Got, &g); err != nil {
			continue
		}
		if _, err := fmt.Sscan(diff.Want, &w); err != nil {
			continue
		}
		if d := abs(g - w); d > worst {
			worst, worstDiff = d, diff
		}
	}

	// Flags live in ECS and physics never writes them, so they must match
	// exactly even after both worlds have simulated on.
	flagDiffs := filterDiffs(harness.CompareCaptures(want, got, driftTol),
		func(d harness.Diff) bool {
			return strings.HasPrefix(d.Field, "Body.") &&
				!strings.HasPrefix(d.Field, "Body.Shapes")
		})
	if len(flagDiffs) == 0 {
		report.Pass(check, "body flags are identical after both worlds simulate on", tick)
	} else {
		report.Fail(check, "body flags are identical after both worlds simulate on", tick,
			"%d flag(s) diverged:\n       %s", len(flagDiffs), joinDiffs(flagDiffs))
	}

	if len(offenders) == 0 {
		report.Pass(check, "a restored world simulates the same as the one it replaced", tick)
		report.Note(check, tick, "no body diverged by more than %.2f m over %d ticks",
			driftTol, driftTicks)
		return
	}
	report.Fail(check, "a restored world simulates the same as the one it replaced", tick,
		"%d body position(s) diverged by more than %.2f m over %d ticks, past what "+
			"a rebuild's lost warm-start impulses can explain. Worst: %s off by "+
			"%.4f m.\n       restored / original:\n       %s",
		len(offenders), driftTol, driftTicks, worstDiff.Label, worst, joinDiffs(offenders))
}

func expectSameBodies(
	report *harness.Report, check string, want, got harness.Capture, tick uint64,
) bool {
	if len(got.Rows) == len(want.Rows) {
		report.Pass(check, "every body is present", tick)
		return true
	}
	var missing []string
	for _, label := range want.Labels() {
		if _, ok := got.Rows[label]; !ok {
			missing = append(missing, label)
		}
	}
	report.Fail(check, "every body is present", tick,
		"restored world has %d bodies, the original had %d; missing: %s",
		len(got.Rows), len(want.Rows), strings.Join(missing, ", "))
	return false
}

func filterDiffs(diffs []harness.Diff, keep func(harness.Diff) bool) []harness.Diff {
	var out []harness.Diff
	for _, d := range diffs {
		if keep(d) {
			out = append(out, d)
		}
	}
	return out
}

func joinDiffs(diffs []harness.Diff) string {
	const maxShown = 25
	lines := make([]string, 0, maxShown)
	for i, d := range diffs {
		if i == maxShown {
			lines = append(lines, fmt.Sprintf("... and %d more", len(diffs)-maxShown))
			break
		}
		lines = append(lines, d.String())
	}
	return strings.Join(lines, "\n       ")
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
