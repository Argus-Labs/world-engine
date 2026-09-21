package scenario

import "github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"

// All returns every scenario in run order. Each gets its own lane in the world, so bodies
// never reach each other and ordering is normally just layout and report order.
//
// The exception is a scenario that acts on the whole world rather than its lane. Reset and
// ShapeSweep both call Plugin.Reset on the same tick, and steps run in this order within a
// tick, so ShapeSweep has to stay last: Reset reads the pre-reset world before dropping it.
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
		ShapeTags(),
		Reset(),
		ShapeSweep(), // must stay after Reset; see above
	}
}
