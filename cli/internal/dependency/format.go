package dependency

import (
	"strings"

	"github.com/argus-labs/world-engine/cli/internal/tui/style"
)

// FormatStatuses returns a formatted status list and help text for missing deps.
func FormatStatuses(statuses []Status) (string, string) {
	var listBuilder, helpBuilder strings.Builder
	for _, s := range statuses {
		if s.Installed {
			listBuilder.WriteString(style.TickIcon.Render() + " " + s.Name + "\n")
		} else {
			listBuilder.WriteString(style.CrossIcon.Render() + " " + s.Name + "\n")
			helpBuilder.WriteString(s.Help + "\n")
		}
	}
	return listBuilder.String(), helpBuilder.String()
}

// FormatMissing returns a formatted block for displaying missing dependencies.
func FormatMissing(statuses []Status) string {
	list, help := FormatStatuses(statuses)
	return style.Container.Render("--- Found Missing Dependencies ---") + "\n\n" + list + "\n" + help + "\n"
}
