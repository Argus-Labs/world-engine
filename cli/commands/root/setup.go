package root

import (
	"github.com/argus-labs/world-engine/cli/commands/root/tui"
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
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}
