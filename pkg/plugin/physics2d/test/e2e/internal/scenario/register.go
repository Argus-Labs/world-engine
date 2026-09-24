package scenario

import "github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"

// All returns every scenario in run order. Each gets its own lane in the world,
// so ordering only affects layout and report ordering, never behaviour.
func All() []harness.Scenario {
	return []harness.Scenario{
		Defaults(),
		Shapes(),
		BodyTypes(),
		Flags(),
		Material(),
		Filtering(),
		Sensors(),
		Contacts(),
		Compound(),
		Queries(),
		Lifecycle(),
		Stability(),
		ShapeEntities(),
		Reset(),
		// After Reset(): its reset-tick step calls Plugin.Reset too, and must run after the
		// reset scenario's own "before" checks when every scenario shares one world.
		ShapeSweep(),
	}
}
