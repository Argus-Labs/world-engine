package steps

import tea "github.com/charmbracelet/bubbletea"

/////////////////////////
// Bubble Tea Commands //
/////////////////////////

type StartMsg struct{}
type CompleteStepMsg struct {
	Err error
}
type SignalStepStartedMsg struct {
	Index int
}
type SignalStepCompletedMsg struct {
	Index int
}
type SignalStepErrorMsg struct {
	Index int
	Err   error
}
type SignalAllStepCompletedMsg struct{}

// StartCmd starts the step component.
func (m Model) StartCmd() tea.Cmd {
	return func() tea.Msg {
		return StartMsg{}
	}
}

// CompleteStepCmd set the current step as completed.
func (m Model) CompleteStepCmd(err error) tea.Cmd {
	return func() tea.Msg {
		return CompleteStepMsg{Err: err}
	}
}

// SignalStepStartedCmd signals the start of a step.
// This is useful for when you want to trigger a certain action when a certain step starts.
func (m Model) SignalStepStartedCmd(index int) tea.Cmd {
	return func() tea.Msg {
		return SignalStepStartedMsg{Index: index}
	}
}

// SignalStepCompletedCmd signals the finish of a step.
// This is useful for when you want to trigger a certain action when a certain step finishes.
func (m Model) SignalStepCompletedCmd(index int) tea.Cmd {
	return func() tea.Msg {
		return SignalStepCompletedMsg{Index: index}
	}
}

// SignalStepErrorCmd signals the finish of a step with an error.
// This is useful for when you want to trigger a certain action when a certain step finishes with an error.
func (m Model) SignalStepErrorCmd(index int, err error) tea.Cmd {
	return func() tea.Msg {
		return SignalStepErrorMsg{Index: index, Err: err}
	}
}

// SignalAllStepCompletedCmd signals the finish of all steps.
func (m Model) SignalAllStepCompletedCmd() tea.Cmd {
	return func() tea.Msg {
		return SignalAllStepCompletedMsg{}
	}
}
