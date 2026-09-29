package tui

import (
	"path/filepath"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/internal/dependency"
	"github.com/argus-labs/world-engine/cli/internal/tui/kit/steps"
	"github.com/argus-labs/world-engine/cli/internal/tui/style"
	"github.com/argus-labs/world-engine/cli/pkg/dnslabel"
	"github.com/argus-labs/world-engine/cli/pkg/worldscaffold"
)

// projectNameErrMsg is shown when a project name isn't dnslabel-canonical (the
// form the cardinal-operator expects for shard paths). Shared by the interactive
// input and the directory-argument path.
const projectNameErrMsg = "Name must use lowercase letters, numbers, and single hyphens (e.g. my-game)"

/////////////////////////
// Bubble Tea Messages //
/////////////////////////

// CloneFinishedMsg signals that the async clone operation has completed.
type CloneFinishedMsg struct {
	Err error
}

// TidyFinishedMsg signals that the async tidy operation has completed.
type TidyFinishedMsg struct {
	Err     error
	Removed bool
}

// NewLogMsg signals a new log line to display.
type NewLogMsg struct {
	Log string
}

// NewLogCmd returns a command that emits a log message.
func NewLogCmd(log string) tea.Cmd {
	return func() tea.Msg {
		return NewLogMsg{Log: log}
	}
}

// TemplateSelectionCompleteMsg indicates template selection is complete.
type TemplateSelectionCompleteMsg struct {
	SelectedTemplate *worldscaffold.GameTemplate
}

//////////////////
// List Adapter //
//////////////////

// templateItem implements list.Item for BubbleTea list component.
type templateItem struct {
	template worldscaffold.GameTemplate
}

func (i templateItem) Title() string       { return i.template.Name }
func (i templateItem) Description() string { return i.template.Description }
func (i templateItem) FilterValue() string { return i.template.Name }

//////////////////////
// Bubble Tea Model //
//////////////////////

// WorldSetupModel is the Bubble Tea model for the setup wizard.
type WorldSetupModel struct {
	logs             []string
	steps            steps.Model
	projectNameInput textinput.Model
	templateList     list.Model
	selectedTemplate *worldscaffold.GameTemplate
	templateFlag     string // Template name from --template flag (validated when step starts)
	showTemplateList bool
	depResult        dependency.CheckResult
	templateErr      error
	nameErr          string // Inline validation error for the project name input
	env              string
}

// NewWorldSetupModel creates a new setup model.
func NewWorldSetupModel(directory, env, templateFlag string) WorldSetupModel {
	pnInput := textinput.New()
	pnInput.Prompt = style.DoubleRightIcon.Render()
	pnInput.Placeholder = "starter-game"
	pnInput.Focus()
	pnInput.Width = 50

	setupSteps := newSetupSteps()

	// Create template list
	templates := worldscaffold.GetAvailableTemplates()
	templateItems := make([]list.Item, len(templates))
	for i, template := range templates {
		templateItems[i] = templateItem{template: template}
	}

	delegate := list.NewDefaultDelegate()
	// List Item Colors
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(lipgloss.Color("#ff8c00"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(lipgloss.Color("#c45c00"))
	// List Selection Indicator
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Border(
		lipgloss.NormalBorder(), false, false, false, true).BorderForeground(lipgloss.Color("#ff8c00"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Border(
		lipgloss.NormalBorder(), false, false, false, true).BorderForeground(lipgloss.Color("#ff8c00"))

	templateList := list.New(templateItems, delegate, 100, 30)
	templateList.SetShowHelp(true)
	templateList.SetShowStatusBar(false)
	templateList.SetFilteringEnabled(true)
	templateList.SetShowTitle(false)

	var templateErr error
	if len(templateItems) == 0 {
		// The templates are deliberately hard coded, so this should never happen
		templateErr = eris.New("no templates available - this is a configuration error")
	}

	nameErr := ""
	if directory != "" {
		// Extract just the directory name from the path
		dirName := filepath.Base(directory)
		pnInput.SetValue(dirName)
		if !dnslabel.IsCanonical(dirName) {
			nameErr = projectNameErrMsg
		}
	}

	return WorldSetupModel{
		steps:            setupSteps,
		projectNameInput: pnInput,
		templateList:     templateList,
		templateFlag:     templateFlag,
		templateErr:      templateErr,
		nameErr:          nameErr,
		env:              env,
	}
}

// Init returns an initial command for the application to run.
func (m WorldSetupModel) Init() tea.Cmd {
	// If there's a template configuration error, quit immediately
	if m.templateErr != nil {
		return tea.Quit
	}
	// If a valid project name was passed as an argument, skip the 1st step. An
	// invalid arg name falls through to the input so the user can fix it (the
	// error is already set on the model).
	if name := m.projectNameInput.Value(); name != "" && dnslabel.IsCanonical(name) {
		return tea.Sequence(textinput.Blink, m.steps.StartCmd(), m.steps.CompleteStepCmd(nil))
	}
	return tea.Sequence(textinput.Blink, m.steps.StartCmd())
}
