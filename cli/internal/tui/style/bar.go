package style

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// progressBarWidth is the fixed character width of the filled+empty portion
// of ProgressBar, excluding the trailing percentage label.
const progressBarWidth = 24

//nolint:gochecknoglobals // read only, initialize once for performance.
var (
	progressBarFilled = lipgloss.NewStyle().Foreground(lipgloss.Color(Yellow))
	progressBarEmpty  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// ProgressBar renders a fixed-width filled/empty block bar plus a
// right-aligned percentage, e.g. "████████░░░░░░░░░░░░░░░░  42%".
func ProgressBar(percent int) string {
	switch {
	case percent < 0:
		percent = 0
	case percent > 100:
		percent = 100
	}
	filled := percent * progressBarWidth / 100
	bar := progressBarFilled.Render(strings.Repeat("█", filled)) +
		progressBarEmpty.Render(strings.Repeat("░", progressBarWidth-filled))
	return fmt.Sprintf("%s %3d%%", bar, percent)
}
