package tui

import (
	"strings"

	"github.com/argus-labs/world-engine/cli/internal/dependency"
	"github.com/argus-labs/world-engine/cli/internal/tui/style"
)

// View renders the UI based on the data in the WorldSetupModel.
func (m WorldSetupModel) View() string {
	if m.depResult.Err != nil {
		return dependency.FormatMissing(m.depResult.Statuses)
	}

	if m.templateErr != nil {
		return style.CrossIcon.Render() + " Error: " + m.templateErr.Error() + "\n"
	}

	output := m.steps.View() + "\n\n"

	// Show template list if we're in template selection mode
	if m.showTemplateList {
		output += "  Choose a starting template for your game\n"
		output += m.templateList.View() + "\n\n"
	} else {
		output += style.QuestionIcon.Render() + "What is your game shard name? "
		output += m.projectNameInput.View() + "\n"
		if m.nameErr != "" {
			output += style.CrossIcon.Render() + m.nameErr + "\n"
		}
		output += "\n"
	}

	output += strings.Join(m.logs, "\n")
	return output
}
