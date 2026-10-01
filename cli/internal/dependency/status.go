package dependency

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"
)

// Status wraps a Dependency with its check result.
type Status struct {
	Dependency

	Installed bool
}

// CheckResult is the Tea message returned after checking dependencies.
type CheckResult struct {
	Statuses []Status
	Err      error
}

// CheckCmd returns a Tea command that checks the given dependencies.
func CheckCmd(deps ...Dependency) tea.Cmd {
	return func() tea.Msg {
		statuses := make([]Status, 0, len(deps))
		var errs error
		for _, dep := range deps {
			err := dep.Check()
			statuses = append(statuses, Status{
				Dependency: dep,
				Installed:  err == nil,
			})
			errs = errors.Join(errs, err)
		}
		return CheckResult{Statuses: statuses, Err: errs}
	}
}
