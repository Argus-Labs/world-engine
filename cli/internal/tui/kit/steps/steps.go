package steps

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/argus-labs/world-engine/cli/internal/tui/style"
)

const (
	INCOMPLETE = iota
	COMPLETE
	FAILED
)

//////////////////////
// Bubble Tea Model //
//////////////////////

type Model struct {
	index   int
	Steps   []Entry
	spinner spinner.Model
}

func New() Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff8c00"))

	return Model{
		index:   0,
		Steps:   []Entry{},
		spinner: s,
	}
}

//////////////////////////
// Bubble Tea Lifecycle //
//////////////////////////

// Init returns an initial command for the application to run.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles incoming events and updates the model accordingly.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case StartMsg:
		return m, m.spinner.Tick
	case CompleteStepMsg:
		if msg.Err != nil {
			m.Steps[m.index].Status = FAILED
			m.Steps[m.index].Err = msg.Err
			// Send a signal that the step has failed with an error
			return m, m.SignalStepErrorCmd(m.index, msg.Err)
		}

		m.Steps[m.index].Status = COMPLETE
		// If this is the last step, then we're done
		// Otherwise, we move on to the next step
		if m.index == len(m.Steps)-1 {
			// Send a signal that all the steps have been finished
			return m, tea.Sequence(m.SignalStepCompletedCmd(m.index), m.SignalAllStepCompletedCmd())
		}
		m.index++
		// Send a signal that the current step has been completed and the next step has started
		return m, tea.Sequence(m.SignalStepCompletedCmd(m.index-1), m.SignalStepStartedCmd(m.index))

	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
}

// View renders the UI based on the data in the model.
func (m Model) View() string {
	output := ""
	var outputSb83 strings.Builder
	for i, step := range m.Steps {
		icon := ""
		switch step.Status {
		case INCOMPLETE:
			// If this is the current step, show the spinner
			// Otherwise, show the to do icon.
			if i == m.index {
				icon = m.spinner.View()
			} else {
				icon = style.TodoIcon.Render()
			}
		case COMPLETE:
			icon = style.TickIcon.Render()
		case FAILED:
			icon = style.CrossIcon.Render()
		}

		fmt.Fprint(&outputSb83, icon, step.Text)
		if i != len(m.Steps)-1 {
			outputSb83.WriteString("\n")
		}
	}
	output += outputSb83.String()
	return style.Container.Render(output)
}
