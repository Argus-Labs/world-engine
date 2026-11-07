package tablepageselect

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rotisserie/eris"

	errorspkg "github.com/argus-labs/world-engine/cli/internal/errors"
	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/internal/tui/kit/program"
)

var (
	// ErrNoItems indicates there were no items to select from.
	ErrNoItems = eris.New("no items to select from")
)

// RunWithHotkeys runs the picker with caller-defined shortcuts. It returns an
// action token when a shortcut closes the picker; an empty token means the user
// selected a row normally.
//
// Errors: ErrNoItems for empty rows, [context.Canceled] on Ctrl+C, silent
// error on esc/abort, wrapped Bubble Tea errors otherwise.
func RunWithHotkeys(
	ctx context.Context,
	prompt string,
	columns []string,
	rows [][]string,
	defaultIndex int,
	extraKeys []ExtraKey,
) (int, string, error) {
	if len(rows) == 0 {
		return -1, "", ErrNoItems
	}

	model := InitialModel(ctx, prompt, columns, rows, defaultIndex)
	model.ExtraKeys = extraKeys

	prog := program.NewTeaProgram(model)
	m, err := prog.Run()
	if err != nil {
		if errors.Is(err, tea.ErrInterrupted) || errors.Is(err, tea.ErrProgramKilled) {
			return -1, "", context.Canceled // SIGINT without a TTY; same exit as ctrl+c in the picker
		}
		return -1, "", eris.Wrap(err, "failed to run paginated select UI")
	}

	// Remove the trailing hint line so the next output starts cleanly.
	printer.MoveCursorUp(1)
	printer.ClearToEndOfLine()

	out, ok := m.(*Model)
	if !ok {
		return -1, "", eris.New("failed to cast paginated select model")
	}
	if out.Canceled {
		return -1, "", context.Canceled
	}
	if out.Aborted {
		return -1, "", errorspkg.NewSilent(eris.New("selection aborted"))
	}
	return out.Cursor, out.HotkeyAction, nil
}
