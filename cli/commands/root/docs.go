package root

import (
	"github.com/pkg/browser"
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/internal/telemetry"
	"github.com/argus-labs/world-engine/cli/internal/tui/component/textinput"
)

const docsURL = "https://world.dev"

type DocsCmd struct{}

func (c *DocsCmd) Run() error {
	telemetry.PosthogCaptureEvent("docs-command", nil)
	// Ask for confirmation before opening browser
	prompt := "Open " + docsURL + " in your browser?"
	confirmed, err := textinput.Confirm(prompt, "y")
	if err != nil {
		return eris.Wrap(err, "failed to confirm")
	}
	if !confirmed {
		printer.Notificationf("Please visit this URL to view the docs: %s", docsURL)
		printer.NewLine(1)
		return nil
	}

	// Open browser
	err = browser.OpenURL(docsURL)
	if err != nil {
		return eris.Wrap(err, "failed to open browser")
	}
	return nil
}
