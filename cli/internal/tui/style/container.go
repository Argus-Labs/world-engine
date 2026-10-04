package style

import "github.com/charmbracelet/lipgloss"

//nolint:gochecknoglobals // This is a normal pattern for style constants in UI libraries
var Container = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1,
	2).BorderForeground(lipgloss.Color("#ff8c00"))
