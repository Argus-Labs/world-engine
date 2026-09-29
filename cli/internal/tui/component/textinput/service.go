package textinput

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/rotisserie/eris"

	errorspkg "github.com/argus-labs/world-engine/cli/internal/errors"
	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/internal/tui/kit/program"
)

const (
	DefaultWidth = 50
	DefaultHint  = "(Enter to submit, esc to quit)"
)

// Prompt executes the text input UI.
func Prompt(
	prompt string,
	defaultValue string,
) (string, error) {
	ti := textinput.New()
	// Configure input box styling - Bubble Tea handles focus/unfocus styling automatically
	ti.Prompt = ""
	// Dim placeholder (default value) appearance
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	// Use defaultValue as placeholder so it appears dim and disappears on typing; reappears when cleared
	if defaultValue != "" {
		ti.Placeholder = defaultValue
	}
	ti.Width = DefaultWidth

	// Start with empty value and focus cursor at index 0
	ti.SetValue("")
	ti.Focus()

	header := fmt.Sprintf("%s:", prompt)
	p := program.NewTeaProgram(model{input: ti, header: header, hint: DefaultHint})
	m, err := p.Run()
	if err != nil {
		return "", err
	}
	mm, ok := m.(model)
	if !ok {
		return "", eris.New("invalid model")
	}
	// Move cursor up one line to avoid extra gap before the next prompt
	printer.MoveCursorUp(1)
	if mm.aborted {
		return "", errorspkg.NewSilent(eris.New("input canceled"))
	}
	return mm.finalValue, nil
}

// Confirm asks for y/n with default and inline error when invalid value entered.
func Confirm(prompt string, defaultValue string) (bool, error) {
	ti := textinput.New()
	ti.Prompt = ""
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	if defaultValue != "" {
		ti.Placeholder = defaultValue
	}
	ti.Width = DefaultWidth
	ti.SetValue("")
	ti.Focus()

	header := fmt.Sprintf("%s:", prompt+" (y/n)")
	validator := func(v string) (bool, string, string) {
		switch strings.ToLower(v) {
		case "y", "yes":
			return true, "y", ""
		case "n", "no":
			return true, "n", ""
		default:
			return false, v, "Must be \"y\" or \"n\""
		}
	}

	p := program.NewTeaProgram(model{input: ti, header: header, hint: DefaultHint, validator: validator})
	m, err := p.Run()
	if err != nil {
		return false, err
	}
	mm, ok := m.(model)
	if !ok {
		return false, eris.New("invalid model")
	}
	printer.MoveCursorUp(1)
	if mm.aborted {
		return false, errorspkg.NewSilent(eris.New("input canceled"))
	}
	return mm.finalValue == "y", nil
}

// PromptValidate executes the text input UI with a custom validator.
// The validator is called when the user presses Enter. If it returns an error,
// the error message is shown inline beneath the input. If it returns a value
// and no error, the potentially-normalized value is accepted.
func PromptValidate(
	prompt string,
	defaultValue string,
	validate func(string) (string, error),
) (string, error) {
	ti := textinput.New()
	ti.Prompt = ""
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	if defaultValue != "" {
		ti.Placeholder = defaultValue
	}
	ti.Width = DefaultWidth
	ti.SetValue("")
	ti.Focus()

	header := fmt.Sprintf("%s:", prompt)

	var wrapper func(string) (bool, string, string)
	if validate != nil {
		wrapper = func(v string) (bool, string, string) {
			normalized, err := validate(v)
			if err != nil {
				return false, v, err.Error()
			}
			return true, normalized, ""
		}
	}

	p := program.NewTeaProgram(model{input: ti, header: header, hint: DefaultHint, validator: wrapper})
	m, err := p.Run()
	if err != nil {
		return "", err
	}
	mm, ok := m.(model)
	if !ok {
		return "", eris.New("invalid model")
	}
	printer.MoveCursorUp(1)
	if mm.aborted {
		return "", errorspkg.NewSilent(eris.New("input canceled"))
	}
	return mm.finalValue, nil
}
