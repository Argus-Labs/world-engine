package root

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

// A box wider than the terminal gets soft-wrapped by it, breaking the borders;
// long lines must wrap inside the box instead, without losing any characters.
func TestEndpointsBox_WrapsToTerminalWidth(t *testing.T) {
	t.Parallel()

	const width = 40
	lines := []string{
		endpointLine("API", "http://localhost:8080/some-organization/a-long-project-name/<instance>"),
		endpointLine("NATS", "nats://127.0.0.1:4222"),
		endpointLine(strings.Repeat("s", 63), "127.0.0.1:8081"),
	}

	var content strings.Builder
	for line := range strings.SplitSeq(ansi.Strip(endpointsBox(lines, width)), "\n") {
		assert.LessOrEqual(t, ansi.StringWidth(line), width, "line overflows the terminal: %q", line)
		if strings.HasPrefix(line, "│") {
			content.WriteString(strings.Trim(line, "│ "))
		}
	}

	squash := func(s string) string { return strings.ReplaceAll(s, " ", "") }
	assert.Equal(t, squash(strings.Join(lines, "")), squash(content.String()))
}

// Without a known terminal width every line stays whole on one row.
func TestEndpointsBox_NoWidthKeepsLinesWhole(t *testing.T) {
	t.Parallel()

	long := endpointLine("API", "http://localhost:8080/some-organization/a-long-project-name/<instance>")
	assert.Contains(t, ansi.Strip(endpointsBox([]string{long}, 0)), long)
}
