// Package probe holds the ECS components this test game adds on top of the
// physics2d plugin's own Transform2D / Velocity2D / PhysicsBody2D.
package probe

import "github.com/goccy/go-json"

// Probe tags every entity the test harness spawns. Scenario is the owning
// scenario's name and Label identifies the body inside that scenario, so failure
// messages can name the exact body that misbehaved instead of a bare entity ID.
//
// Wire methods are hand-written rather than generated: this project intentionally
// does not run `world sdk generate`, so nothing here depends on Docker/protoc.
type Probe struct {
	Scenario string `json:"scenario"`
	Label    string `json:"label"`
}

// Name returns the ECS component name.
func (Probe) Name() string { return "probe" }

// MarshalWire encodes the probe for snapshots and debug introspection. It panics
// on a marshal error, the same contract as the generated wire code.
func (p Probe) MarshalWire() []byte {
	b, err := json.Marshal(p)
	if err != nil {
		panic("failed to marshal Probe: " + err.Error())
	}
	return b
}

// UnmarshalWire decodes a probe produced by MarshalWire.
func (Probe) UnmarshalWire(b []byte) (any, error) {
	var v Probe
	err := json.Unmarshal(b, &v)
	return v, err
}
