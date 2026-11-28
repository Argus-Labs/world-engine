package root

import (
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/commands/root/tui"
	errorspkg "github.com/argus-labs/world-engine/cli/internal/errors"
	"github.com/argus-labs/world-engine/cli/internal/telemetry"
	"github.com/argus-labs/world-engine/cli/internal/tui/kit/program"
)

type SetupCmd struct {
	Directory string `arg:"" optional:"" type:"path" help:"The directory to create the project in (name must be lowercase letters, numbers, and hyphens)"`
	Template  string `                               help:"Template to use (skips interactive selection). Options: basic, demo, bare"                     short:"t"`
}

func (c *SetupCmd) Run(rootCmd *Cmd) error {
	telemetry.PosthogCaptureEvent("setup-command", map[string]any{
		"directory": c.Directory,
		"template":  c.Template,
	})

	p := program.NewTeaProgram(tui.NewWorldSetupModel(c.Directory, rootCmd.GetEnv(), c.Template))

	final, err := p.Run()
	if err != nil {
		return err
	}

	m, ok := final.(tui.WorldSetupModel)
	if !ok {
		return eris.New("invalid setup model")
	}

	// The wizard has already rendered the failure; returning it only sets the exit code.
	return errorspkg.NewSilent(m.Err())
}
