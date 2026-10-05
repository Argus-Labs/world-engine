package tui

import (
	"github.com/argus-labs/world-engine/cli/internal/tui/kit/steps"
)

// setupStep identifies each step in the setup wizard.
type setupStep int

const (
	stepName setupStep = iota
	stepTemplate
	stepClone
	stepTidy
)

func newSetupSteps() steps.Model {
	s := steps.New()
	s.Steps = []steps.Entry{
		steps.NewStep("Set game shard name"),
		steps.NewStep("Choose a starting template for your game"),
		steps.NewStep("Initialize game shard with selected template"),
		steps.NewStep("Tidy template go.mod dependencies"),
	}
	return s
}
