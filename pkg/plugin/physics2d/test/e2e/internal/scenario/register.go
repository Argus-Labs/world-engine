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
		Reset(),
	}
}
