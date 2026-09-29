package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/argus-labs/world-engine/cli/internal/dependency"
	"github.com/argus-labs/world-engine/cli/internal/tui/kit/steps"
	"github.com/argus-labs/world-engine/cli/internal/tui/style"
	"github.com/argus-labs/world-engine/cli/pkg/dnslabel"
	"github.com/argus-labs/world-engine/cli/pkg/version"
	"github.com/argus-labs/world-engine/cli/pkg/worldscaffold"
)

// Update handles incoming events and updates the model accordingly.
func (m WorldSetupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case NewLogMsg:
		return m.handleLog(msg)
	case dependency.CheckResult:
		return m.handleDepResult(msg)
	case steps.SignalStepStartedMsg:
		return m.handleStepStarted(msg)
	case steps.SignalStepCompletedMsg:
		return m.handleStepCompleted(msg)
	case steps.SignalStepErrorMsg:
		return m.handleStepError(msg)
	case steps.SignalAllStepCompletedMsg:
		return m, tea.Quit
	case CloneFinishedMsg:
		return m.handleCloneFinished(msg)
	case TidyFinishedMsg:
		return m.handleTidyFinished(msg)
	case tea.WindowSizeMsg:
		return m.handleResize(msg)
	}
	return m.updateSteps(msg)
}

func (m WorldSetupModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}

	switch {
	case m.showTemplateList:
		return m.handleTemplateKey(msg)
	case m.projectNameInput.Focused():
		return m.handleNameInputKey(msg)
	default:
		return m, nil // Clone/tidy running — ignore keypresses
	}
}

func (m WorldSetupModel) handleNameInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEnter {
		if m.projectNameInput.Value() == "" {
			m.projectNameInput.SetValue("starter-game")
		}
		name := m.projectNameInput.Value()
		// Match the cardinal-editor: the name must already be dnslabel-canonical so
		// world.toml's project (and the shard paths the operator derives from it)
		// address cleanly with no surprising normalization.
		if !dnslabel.IsCanonical(name) {
			m.nameErr = projectNameErrMsg
			return m, nil
		}
		m.nameErr = ""
		m.projectNameInput.Blur()
		return m, m.steps.CompleteStepCmd(nil)
	}

	// Clear validation error when the user edits the input
	m.nameErr = ""
	var cmd tea.Cmd
	m.projectNameInput, cmd = m.projectNameInput.Update(msg)
	return m, cmd
}

func (m WorldSetupModel) handleTemplateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.templateList, cmd = m.templateList.Update(msg)

	if msg.Type == tea.KeyEnter {
		if selectedItem := m.templateList.SelectedItem(); selectedItem != nil {
			if item, ok := selectedItem.(templateItem); ok {
				m.selectedTemplate = &item.template
				m.showTemplateList = false
				return m, m.steps.CompleteStepCmd(nil)
			}
		}
	}

	return m, cmd
}

func (m WorldSetupModel) handleLog(msg NewLogMsg) (tea.Model, tea.Cmd) {
	m.logs = append(m.logs, msg.Log)
	return m, nil
}

func (m WorldSetupModel) handleDepResult(msg dependency.CheckResult) (tea.Model, tea.Cmd) {
	m.depResult = msg
	if msg.Err != nil {
		return m, tea.Quit
	}
	return m, nil
}

func (m WorldSetupModel) handleStepStarted(msg steps.SignalStepStartedMsg) (tea.Model, tea.Cmd) {
	switch setupStep(msg.Index) { //nolint:exhaustive // Do not need to handle stepName here
	case stepTemplate:
		// If template was specified via --template flag, validate and use it
		if m.templateFlag != "" {
			template, err := worldscaffold.GetTemplateByName(m.templateFlag)
			if err != nil {
				return m, tea.Sequence(
					NewLogCmd(style.CrossIcon.Render()+err.Error()),
					tea.Quit,
				)
			}
			m.selectedTemplate = template
			return m, tea.Sequence(
				NewLogCmd(style.ChevronIcon.Render()+"Using template: "+m.selectedTemplate.Name),
				m.steps.CompleteStepCmd(nil),
			)
		}
		m.showTemplateList = true
		return m, NewLogCmd(fmt.Sprintf(
			style.ChevronIcon.Render()+"Loaded %d templates for selection",
			len(m.templateList.Items()),
		))

	case stepClone:
		if m.selectedTemplate == nil {
			return m, tea.Sequence(
				NewLogCmd(style.CrossIcon.Render()+"No template selected"),
				tea.Quit,
			)
		}
		return m, tea.Sequence(
			NewLogCmd(style.ChevronIcon.Render()+"Cloning "+m.selectedTemplate.Name+"..."),
			m.cloneTemplateCmd(),
		)

	case stepTidy:
		return m, tea.Sequence(
			NewLogCmd(style.ChevronIcon.Render()+"Tidying template go.mod dependencies..."),
			m.tidyCmd(),
		)
	}
	return m, nil
}

func (m WorldSetupModel) handleStepCompleted(msg steps.SignalStepCompletedMsg) (tea.Model, tea.Cmd) {
	switch setupStep(msg.Index) { //nolint:exhaustive // Only specific steps need logging
	case stepTemplate:
		if m.selectedTemplate != nil {
			return m, NewLogCmd(style.ChevronIcon.Render() +
				"Selected template: " + m.selectedTemplate.Name)
		}
	case stepClone:
		return m, NewLogCmd(style.ChevronIcon.Render() +
			"Successfully created a starter game shard in ./" + m.projectNameInput.Value())
	}
	return m, nil
}

func (m WorldSetupModel) handleStepError(msg steps.SignalStepErrorMsg) (tea.Model, tea.Cmd) {
	return m, tea.Sequence(
		NewLogCmd(style.CrossIcon.Render()+"Error: "+msg.Err.Error()),
		tea.Quit,
	)
}

func (m WorldSetupModel) handleCloneFinished(msg CloneFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.logs = append(m.logs, style.CrossIcon.Render()+msg.Err.Error())
		return m, m.steps.CompleteStepCmd(msg.Err)
	}
	return m, m.steps.CompleteStepCmd(nil)
}

func (m WorldSetupModel) handleTidyFinished(msg TidyFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.logs = append(m.logs, style.CrossIcon.Render()+msg.Err.Error())
		if msg.Removed {
			m.logs = append(m.logs, style.ChevronIcon.Render()+
				"Removed ./"+m.projectNameInput.Value()+" — fix the issue above and run setup again with the same name.")
		}
		return m, m.steps.CompleteStepCmd(msg.Err)
	}
	return m, m.steps.CompleteStepCmd(nil)
}

func (m WorldSetupModel) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	const (
		// Rough allowance for header text, spacing, and a couple of log lines.
		reservedHeaderAndLogsLines = 6
		// Don't shrink the list below a few rows so selection is still usable.
		minTemplateListHeight = 3
	)

	// Count how many lines the steps container currently renders.
	stepsLines := strings.Count(m.steps.View(), "\n") + 1 // +1 for the last line missing a newline

	// Whatever is left goes to the template list.
	available := max(msg.Height-stepsLines-reservedHeaderAndLogsLines, minTemplateListHeight)

	// Width uses msg.Width minus 4 to match the inner content width of
	// style.Container, which has horizontal padding of 2 on each side.
	m.templateList.SetSize(msg.Width-4, available)
	return m, nil
}

func (m WorldSetupModel) updateSteps(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.steps, cmd = m.steps.Update(msg)
	return m, cmd
}

// cloneTemplateCmd returns a tea.Cmd that performs the clone operation.
func (m WorldSetupModel) cloneTemplateCmd() tea.Cmd {
	return func() tea.Msg {
		err := worldscaffold.InstantiateTemplate(
			context.Background(),
			m.selectedTemplate.URL,
			version.WorldEngine(),
			m.projectNameInput.Value(),
			m.selectedTemplate.Subdir,
		)
		return CloneFinishedMsg{Err: err}
	}
}

// tidyCmd returns a tea.Cmd that performs the go mod tidy operation.
func (m WorldSetupModel) tidyCmd() tea.Cmd {
	return func() tea.Msg {
		projectDir := m.projectNameInput.Value()
		err := worldscaffold.Tidy(context.Background(), projectDir)
		removed := false
		if err != nil {
			// Match the editor: a failed tidy leaves nothing behind so the name is
			// retryable (the project directory was created by InstantiateTemplate).
			removed = os.RemoveAll(projectDir) == nil
		}
		return TidyFinishedMsg{Err: err, Removed: removed}
	}
}
