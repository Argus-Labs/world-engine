package root

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/internal/tui/component/phasebox"
	tomlpkg "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// loadWorldConfig reads world.toml from the working directory.
func loadWorldConfig() (tomlpkg.Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return tomlpkg.Config{}, eris.Wrap(err, "failed to get current directory")
	}
	return tomlpkg.LoadFile(cwd + "/" + tomlpkg.FileName)
}

// runSingleStep runs one lifecycle op (`world stop`, `world purge`) through a
// one-row phasebox section; rowLabel + resultVerb becomes the collapsed summary
// (e.g. "stopped — rampage (2s)").
func runSingleStep(
	ctx context.Context,
	project, rowID, rowLabel, resultVerb string,
	op func(ctx context.Context) error,
) error {
	dash := phasebox.Start(ctx, phasebox.TTY)
	defer dash.Complete()
	return dash.Run("World",
		func(ctx context.Context, sess phasebox.Session) error {
			sess.UpsertRow(rowID, rowLabel, "", phasebox.Active)
			if opErr := op(ctx); opErr != nil {
				sess.Fail(rowID, rowLabel, opErr)
				return opErr
			}
			sess.UpsertRow(rowID, rowLabel, "", phasebox.Done)
			return nil
		},
		func(elapsed time.Duration) string {
			return fmt.Sprintf("%s — %s (%s)", resultVerb, project, elapsed.Round(time.Second))
		},
	)
}
