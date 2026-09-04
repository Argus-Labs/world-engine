// Command game is the physics2d test game: a Cardinal world whose only job is to
// exercise the physics2d plugin and report anything that does not behave the way
// Box2D does.
//
// By default it runs headless — it drives World.Tick itself, needs no NATS, no
// Redis and no Docker, prints a pass/fail report and exits non-zero if any check
// failed. With -serve it starts as a normal Cardinal shard instead, for running
// inside the world stack.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"
	restorepkg "github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/restore"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/scenario"

	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

func main() {
	var (
		serve   = flag.Bool("serve", false, "run as a normal Cardinal shard instead of the headless checker")
		verbose = flag.Bool("v", false, "print passing checks as well as failures")
		gravity = flag.Float64("gravity", -10, "world gravity on Y")
		substep = flag.Int("substeps", 4, "physics solver sub-steps per step")
		workers = flag.Int("workers", 0, "physics engine worker count; results must not depend on it")
		extra   = flag.Uint64("extra-ticks", 0, "run this many ticks beyond the last scheduled check")
		digest  = flag.Bool("digest", false, "print a hash of every body's final state; two runs must match")
		hostile = flag.String("hostile", "", "run one crash-prone case alone instead of the suite (see -hostile list)")
		restore = flag.Bool("restore", false, "run the crash-restore check instead of the suite")
		noReset = flag.Bool("restore-no-reset", false, "with -restore, skip the documented Plugin.Reset after FromProto")
	)
	flag.Parse()

	// Surface plugin-side problems: the physics pipeline logs reconcile and
	// rebuild failures at error level rather than returning them.
	if os.Getenv("LOG_LEVEL") == "" {
		_ = os.Setenv("LOG_LEVEL", "error")
	}

	if *restore {
		os.Exit(restorepkg.Run(harness.Config{
			Gravity:      physics.Vec2{X: 0, Y: *gravity},
			SubStepCount: *substep,
			Verbose:      *verbose,
		}, !*noReset))
	}

	scenarios := scenario.All()
	if *hostile != "" {
		if *hostile == "list" {
			// Names only on stdout so the list can be piped into a loop; the
			// explanation goes to stderr.
			fmt.Fprintln(os.Stderr, "crash-prone cases; each runs alone and may "+
				"terminate the process, which is the result being measured:")
			for _, name := range scenario.HostileNames() {
				fmt.Println(name)
			}
			return
		}
		sc, ok := scenario.Hostile(*hostile)
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown -hostile case %q; try -hostile list\n", *hostile)
			os.Exit(2)
		}
		scenarios = []harness.Scenario{sc}
	}

	cfg := harness.Config{
		Gravity:      physics.Vec2{X: 0, Y: *gravity},
		SubStepCount: *substep,
		Workers:      *workers,
		Verbose:      *verbose,
		ExtraTicks:   *extra,
		Digest:       *digest,
	}

	runner := harness.New(scenarios, cfg)
	world, err := runner.BuildWorld(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build world: %v\n", err)
		os.Exit(2)
	}

	if *serve {
		// StartGame runs the same systems on a wall-clock ticker and needs the
		// usual shard infrastructure (NATS). Checks stream to stdout as they
		// fire; the summary prints when the shard is stopped, because StartGame
		// keeps ticking long after the last scheduled check.
		fmt.Printf("serving as a Cardinal shard; the %d scheduled ticks take about "+
			"%.0fs, then stop the shard to see the summary\n",
			runner.LastTick()+1, float64(runner.LastTick()+1)/harness.TickRate)
		world.StartGame()
		runner.Report().Print()
		if _, fail, _ := runner.Report().Totals(); fail > 0 {
			os.Exit(1)
		}
		return
	}

	os.Exit(runner.Run(world))
}
