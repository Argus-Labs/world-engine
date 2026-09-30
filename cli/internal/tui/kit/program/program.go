package program

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

// Program An interface describing the parts of BubbleTea's Program that we actually use.
type Program interface {
	Start() error
	Quit()
}

type (
	StatusMsg   string
	ProgressMsg *float64
	PsqlMsg     *string
)

type StatusWriter struct {
	Program
}

// NewTeaProgram will create a BubbleTea program that automatically sets the no input option
// if you are not on a TTY, so you can run the debugger. Call it just as you would call tea.NewProgram().
func NewTeaProgram(model tea.Model, opts ...tea.ProgramOption) *tea.Program {
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		opts = append(opts, tea.WithInput(nil))
		// opts = append(opts, tea.WithoutRenderer())
	}
	return tea.NewProgram(model, opts...)
}
