package root

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bodyLines returns the box's content lines, borders stripped.
func bodyLines(box string) []string {
	var out []string
	for line := range strings.SplitSeq(ansi.Strip(box), "\n") {
		if strings.HasPrefix(line, "│") {
			out = append(out, strings.TrimSuffix(strings.TrimPrefix(line, "│ "), " │"))
		}
	}
	return out
}

// A box wider than the terminal gets soft-wrapped by it, breaking the borders;
// long lines must wrap inside the box instead, without losing any characters.
func TestEndpointsBox_WrapsToTerminalWidth(t *testing.T) {
	t.Parallel()

	const width = 40
	eps := []endpoint{
		{"API", "http://localhost:8080/some-organization/a-long-project-name/<instance>"},
		{"NATS", "nats://127.0.0.1:4222"},
		{strings.Repeat("s", 63), "127.0.0.1:8081"},
	}

	var content strings.Builder
	for line := range strings.SplitSeq(ansi.Strip(endpointsBox(eps, width)), "\n") {
		assert.LessOrEqual(t, ansi.StringWidth(line), width, "line overflows the terminal: %q", line)
		if strings.HasPrefix(line, "│") {
			content.WriteString(strings.Trim(line, "│ "))
		}
	}

	var want strings.Builder
	for _, e := range eps {
		want.WriteString(e.label + ":" + e.addr)
	}
	squash := func(s string) string { return strings.ReplaceAll(s, " ", "") }
	assert.Equal(t, squash(want.String()), squash(content.String()))
}

// Without a known terminal width every line stays whole on one row.
func TestEndpointsBox_NoWidthKeepsLinesWhole(t *testing.T) {
	t.Parallel()

	addr := "http://localhost:8080/some-organization/a-long-project-name/<instance>"
	assert.Contains(t, ansi.Strip(endpointsBox([]endpoint{{"API", addr}}, 0)), "API: "+addr)
}

// Instance IDs are arbitrarily long, so the address column is sized to the
// longest label rather than fixed; otherwise the shard rows step rightwards.
func TestEndpointsBox_AlignsAddressColumn(t *testing.T) {
	t.Parallel()

	eps := []endpoint{{"API", "a1"}, {"gameplay-2", "a2"}, {"meta", "a3"}}

	lines := bodyLines(endpointsBox(eps, 0))
	require.Len(t, lines, len(eps))
	for i, line := range lines {
		assert.Equal(t, strings.Index(lines[0], "a1"), strings.Index(line, eps[i].addr),
			"address column moved on %q", line)
	}
}
