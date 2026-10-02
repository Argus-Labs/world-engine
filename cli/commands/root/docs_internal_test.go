package root

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/term"
)

// captureStdout swaps os.Stdout for a pipe, runs fn, and returns whatever was
// written. Tests using it must not call t.Parallel: os.Stdout is process-global
// and the package's other test files run parallel tests.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	//nolint:reassign // deliberate, scoped, and restored by the defer below
	os.Stdout = w
	//nolint:reassign // restores what the line above swapped
	defer func() { os.Stdout = orig }()

	runErr := fn()
	_ = w.Close()

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r) // returns once w is closed
	_ = r.Close()
	return buf.String(), runErr
}

// Reproduces the bug: when stderr is not a TTY, program.NewTeaProgram attaches
// no input reader, so textinput.Confirm would stall forever waiting for a
// keypress it can never receive. The guard must skip the prompt, print the URL,
// and return nil — never starting Bubble Tea or opening the browser.
func TestDocsRunPrintsURLWithoutPromptingWhenStderrNotATTY(t *testing.T) {
	promptCalls := 0
	browserCalls := 0
	c := &DocsCmd{
		canPromptFn:   func() bool { return false },
		confirmFn:     func(string, string) (bool, error) { promptCalls++; return false, nil },
		openBrowserFn: func(string) error { browserCalls++; return nil },
	}

	out, err := captureStdout(t, c.Run)
	require.NoError(t, err)
	require.Zero(t, promptCalls, "the Bubble Tea confirm must not start when stderr is not a TTY")
	require.Zero(t, browserCalls, "the browser must not open when stderr is not a TTY")
	require.Contains(t, out, "Please visit this URL to view the docs")
	require.Contains(t, out, docsURL)
}

// Interactive happy path: stderr is a TTY, the user confirms "y", the browser
// opens with the docs URL. Guards the wiring between the prompt and the
// browser-open call against regressions from the non-TTY guard above.
func TestDocsRunOpensBrowserWhenConfirmedWithTTY(t *testing.T) {
	promptCalls := 0
	browserCalls := 0
	var promptArg, browserArg string
	c := &DocsCmd{
		canPromptFn: func() bool { return true },
		confirmFn: func(prompt, def string) (bool, error) {
			promptCalls++
			promptArg = prompt
			require.Equal(t, "y", def, "default must be \"y\"")
			return true, nil
		},
		openBrowserFn: func(url string) error {
			browserCalls++
			browserArg = url
			return nil
		},
	}

	require.NoError(t, c.Run())
	require.Equal(t, 1, promptCalls)
	require.Equal(t, 1, browserCalls)
	require.Contains(t, promptArg, docsURL, "prompt must name the docs URL")
	require.Contains(t, promptArg, "browser", "prompt must mention the browser")
	require.Equal(t, docsURL, browserArg)
}

// The production canPrompt fallback (nil canPromptFn) must consult os.Stderr,
// matching program.NewTeaProgram's gate exactly — not os.Stdin or os.Stdout.
// Two reasons it must match: a mismatch would either prompt when there is no
// input reader (hang, the original bug) or skip the prompt when the prompt
// would have worked (breaks the auto-open path).
func TestDocsCanPromptDefaultsToStderrTTYCheck(t *testing.T) {
	c := &DocsCmd{}
	want := term.IsTerminal(int(os.Stderr.Fd()))
	require.Equal(t, want, c.canPrompt(), "canPrompt must gate on os.Stderr, matching NewTeaProgram")
}
