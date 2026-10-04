package style

import "github.com/charmbracelet/lipgloss"

//nolint:gochecknoglobals // This is a normal pattern for style constants in UI libraries
var (
	QuestionIcon    = lipgloss.NewStyle().Foreground(lipgloss.Color("251")).SetString("? ").Bold(true)
	CrossIcon       = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).SetString("✗ ").Bold(true)
	TickIcon        = lipgloss.NewStyle().Foreground(lipgloss.Color(Yellow)).SetString("✓ ").Bold(true)
	TodoIcon        = lipgloss.NewStyle().SetString("• ").Bold(true)
	DoubleRightIcon = lipgloss.NewStyle().Foreground(lipgloss.Color("251")).SetString("» ").Bold(true)
	ChevronIcon     = lipgloss.NewStyle().Foreground(lipgloss.Color("251")).SetString("› ").Bold(true)
)
