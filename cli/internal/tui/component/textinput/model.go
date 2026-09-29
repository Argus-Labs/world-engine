package textinput

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/guumaster/logsymbols"
)

// model is a small Bubble Tea model that presents a focused text input
// and quits on enter or abort keys.
type model struct {
	input      textinput.Model
	aborted    bool
	header     string
	hint       string
	finalized  bool
	finalValue string
	validator  func(string) (bool, string, string)
	errorMsg   string
}

func (m model) Init() tea.Cmd { return textinput.Blink }

//nolint:gocritic // dont want switch statement
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			// Capture final value (use placeholder if empty) and render once without the input box
			val := m.input.Value()
			if val == "" {
				val = m.input.Placeholder
			}
			if m.validator != nil {
				ok, normalized, msg := m.validator(val)
				if !ok {
					m.errorMsg = msg
					return m, nil
				}
				val = normalized
			}
			m.finalized = true
			m.finalValue = val
			return m, tea.Quit
		case "esc", "ctrl+c":
			m.aborted = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

//nolint:nestif // short and sweet
func (m model) View() string {
	view := ""
	// Show header with current input value (or final committed value)
	if m.header != "" {
		// Extract just the prompt part (up to the last colon). This avoids truncating
		// prompts that contain URLs like https://example.com.
		prompt := m.header
		if idx := strings.LastIndex(m.header, ":"); idx != -1 {
			prompt = m.header[:idx+1]
		}
		if m.finalized {
			view += prompt + " " + m.finalValue + "\n\n"
		} else {
			view += prompt + " " + m.input.View() + "\n"
			// If there's no error, insert a blank line before the hint
			if m.errorMsg == "" {
				view += "\n"
			}
		}
	}
	if !m.finalized && m.errorMsg != "" {
		view += string(logsymbols.Error) + " Error: " + m.errorMsg + "\n"
	}
	if m.hint != "" {
		view += m.hint
	}
	return view
}
