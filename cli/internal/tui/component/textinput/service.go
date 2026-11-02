package textinput

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/rotisserie/eris"
	"golang.org/x/term"

	errorspkg "github.com/argus-labs/world-engine/cli/internal/errors"
	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/internal/tui/kit/program"
)

const (
	DefaultWidth = 50
	DefaultHint  = "(Enter to submit, esc to quit)"
)

// Confirm asks for y/n with default and inline error when invalid value entered.
func Confirm(prompt string, defaultValue string) (bool, error) {
	// NewTeaProgram reads no input without a stderr TTY, so the prompt could never be answered.
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		return false, nil
	}
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
