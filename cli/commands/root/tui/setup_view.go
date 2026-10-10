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

	// The live-input section is gated on the wizard's active step, not just on
	// showTemplateList. After template selection the wizard advances to the
	// async stepClone/stepTidy phases; rendering the (already-completed) name
	// step's prompt there would show a stale, and on the arg path editable,
	// input below the step list for the rest of the run.
	switch {
	case m.showTemplateList:
		output += "  Choose a starting template for your game\n"
		output += m.templateList.View() + "\n\n"
	case m.steps.CurrentIndex() == int(stepName):
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
