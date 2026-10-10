package root

import (
	"os"

	"github.com/pkg/browser"
	"github.com/rotisserie/eris"
	"golang.org/x/term"

	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/internal/telemetry"
	"github.com/argus-labs/world-engine/cli/internal/tui/component/textinput"
)

const docsURL = "https://world.dev"

type DocsCmd struct {
	// canPromptFn reports whether the confirmation prompt can run. The zero
	// value falls back to the production check (stderr is a terminal), matching
	// program.NewTeaProgram, which drops the input reader when stderr is not a
	// terminal: a prompt started there would hang with no way to receive the
	// keypress that dismisses it. Tests set it to drive both branches without
	// depending on the test runner's own TTY.
	canPromptFn func() bool

	// confirmFn asks the user a y/n question. The zero value falls back to
	// textinput.Confirm. Tests stub it so the Bubble Tea program — which has
	// no input in a test harness — never starts.
	confirmFn func(prompt string, defaultValue string) (bool, error)

	// openBrowserFn opens a URL in the user's browser. The zero value falls
	// back to browser.OpenURL. Tests stub it to avoid launching a real browser.
	openBrowserFn func(url string) error
}

func (c *DocsCmd) Run() error {
	telemetry.PosthogCaptureEvent("docs-command", nil)

	if !c.canPrompt() {
		// program.NewTeaProgram drops the input reader when stderr is not a
		// terminal, so textinput.Confirm would stall with no way to receive
		// the keypress that dismisses it. Print the URL instead — mirroring
		// auth.go's canPrompt() guard.
		printer.Notificationf("Please visit this URL to view the docs: %s", docsURL)
		printer.NewLine(1)
		return nil
	}

	confirmed, err := c.confirm("Open "+docsURL+" in your browser?", "y")
	if err != nil {
		return eris.Wrap(err, "failed to confirm")
	}
	if !confirmed {
		printer.Notificationf("Please visit this URL to view the docs: %s", docsURL)
		printer.NewLine(1)
		return nil
	}

	if err := c.openBrowser(docsURL); err != nil {
		return eris.Wrap(err, "failed to open browser")
	}
	return nil
}

func (c *DocsCmd) canPrompt() bool {
	if c.canPromptFn != nil {
		return c.canPromptFn()
	}
	return term.IsTerminal(int(os.Stderr.Fd()))
}

func (c *DocsCmd) confirm(prompt, defaultValue string) (bool, error) {
	if c.confirmFn != nil {
		return c.confirmFn(prompt, defaultValue)
	}
	return textinput.Confirm(prompt, defaultValue)
}

func (c *DocsCmd) openBrowser(url string) error {
	if c.openBrowserFn != nil {
		return c.openBrowserFn(url)
	}
	return browser.OpenURL(url)
}
