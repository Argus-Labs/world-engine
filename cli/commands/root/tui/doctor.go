package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/argus-labs/world-engine/cli/internal/dependency"
	"github.com/argus-labs/world-engine/cli/internal/tui/style"
)

//////////////////////
// Bubble Tea Model //
//////////////////////

type WorldDoctorModel struct {
	result dependency.CheckResult
}

func NewWorldDoctorModel() WorldDoctorModel {
	return WorldDoctorModel{}
}

//////////////////////////
// Bubble Tea Lifecycle //
//////////////////////////

// Init returns an initial command for the application to run.
func (m WorldDoctorModel) Init() tea.Cmd {
	return dependency.CheckCmd(
		dependency.Git,
		dependency.Go,
		dependency.Docker,
		dependency.DockerDaemon,
	)
}

// Update handles incoming events and updates the model accordingly.
func (m WorldDoctorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
	case dependency.CheckResult:
		m.result = msg
		return m, tea.Quit
	}
	return m, nil
}

// View renders the model to the screen.
func (m WorldDoctorModel) View() string {
	list, help := dependency.FormatStatuses(m.result.Statuses)
	out := style.Container.Render("--- World CLI Doctor ---") + "\n\n"
	out += "Checking dependencies...\n"
	out += list + "\n" + help + "\n"
	return out
}
