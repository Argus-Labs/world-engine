// Package probe holds the ECS components this test game adds on top of the
// physics2d plugin's own Transform2D / Velocity2D / PhysicsBody2D.
package probe

// Probe tags every entity the test harness spawns. Scenario is the owning
// scenario's name and Label identifies the body inside that scenario, so failure
// messages can name the exact body that misbehaved instead of a bare entity ID.
type Probe struct {
	Scenario string `json:"scenario"`
	Label    string `json:"label"`
}

// Name returns the ECS component name.
func (Probe) Name() string { return "probe" }
