package root

import (
	"github.com/argus-labs/world-engine/cli/commands/root/tui"
	"github.com/argus-labs/world-engine/cli/internal/telemetry"
	"github.com/argus-labs/world-engine/cli/internal/tui/kit/program"
)

type DoctorCmd struct{}

func (c *DoctorCmd) Run() error {
	telemetry.PosthogCaptureEvent("doctor-command", nil)
	p := program.NewTeaProgram(tui.NewWorldDoctorModel())
	_, err := p.Run()
	return err
}
