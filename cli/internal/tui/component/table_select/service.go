package tableselect

import (
	"context"

	"github.com/rotisserie/eris"

	errorspkg "github.com/argus-labs/world-engine/cli/internal/errors"
	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/internal/tui/kit/program"
)

var (
	// ErrNoItems indicates there were no items to select from.
	ErrNoItems = eris.New("no items to select from")
)

// Run executes the numeric selection UI.
func Run(
	ctx context.Context,
	prompt string,
	columns []string,
	rows [][]string,
	defaultIndex int,
) (int, error) {
	if len(rows) == 0 {
		return -1, ErrNoItems
	}

	prog := program.NewTeaProgram(InitialModel(ctx, prompt, columns, rows, defaultIndex))
	m, err := prog.Run()
	if err != nil {
		return -1, eris.Wrap(err, "failed to run numeric select UI")
	}

	// Remove the trailing hint line so the next output starts cleanly
	printer.MoveCursorUp(1)
	printer.ClearToEndOfLine()

	model, ok := m.(*Model)
	if !ok {
		return -1, eris.New("failed to cast numeric select model")
	}
	if model.Aborted {
		return -1, errorspkg.NewSilent(eris.New("selection aborted"))
	}
	return model.Cursor, nil
}
