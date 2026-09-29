package style

import (
	"github.com/charmbracelet/lipgloss"
)

func ForegroundPrint(text string, color string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(text)
}
