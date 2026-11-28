package root

import (
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/commands/root/tui"
	errorspkg "github.com/argus-labs/world-engine/cli/internal/errors"
	"github.com/argus-labs/world-engine/cli/internal/telemetry"
	"github.com/argus-labs/world-engine/cli/internal/tui/kit/program"
)

type DoctorCmd struct{}

func (c *DoctorCmd) Run() error {
	telemetry.PosthogCaptureEvent("doctor-command", nil)

	p := program.NewTeaProgram(tui.NewWorldDoctorModel())

	final, err := p.Run()
	if err != nil {
		return err
	}

	m, ok := final.(tui.WorldDoctorModel)
	if !ok {
		return eris.New("invalid doctor model")
	}

	// The doctor has already rendered its result; returning the error only sets the exit code.
	return errorspkg.NewSilent(m.Err())
}
