package root

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/argus-labs/world-engine/cli/internal/tui/component/phasebox"
	"github.com/argus-labs/world-engine/cli/pkg/cluster"
)

// runSingleStepCluster runs a single named cluster lifecycle op (`world
// stop`'s Stop, `world purge`'s Purge) through a one-row "Cluster" phasebox
// section: rowLabel + resultVerb becomes the collapsed summary (e.g.
// "stopped — world-engine (6s)"). The row goes Active -> Done/Failed around
// op, with k3d's logger routed to it and reset right after op returns (see
// cluster.Client.ResetLogRouting for why that can't wait).
//
// Returns the constructed cluster.Client (callers like `world purge
// --image` need Config()) and any error; the dashboard is always Completed
// first, so callers can print plain output right after.
func runSingleStepCluster(
	ctx context.Context,
	rowID, rowLabel, resultVerb string,
	op func(ctx context.Context, cli *cluster.Client) error,
) (*cluster.Client, error) {
	dash := phasebox.Start(ctx)
	defer dash.Complete()
	var cli *cluster.Client
	err := dash.Run("Cluster",
		func(ctx context.Context, sess phasebox.Session) error {
			cli = cluster.NewClient(cluster.Config{
				LogLevel: os.Getenv("WORLD_K3D_LOG_LEVEL"),
				OnK3DLog: phasebox.ClusterLogRow(sess, rowID, rowLabel),
			})
			sess.UpsertRow(rowID, rowLabel, "", phasebox.Active)
			opErr := op(ctx, cli)
			cli.ResetLogRouting()
			if opErr != nil {
				sess.Fail(rowID, rowLabel, opErr)
				return opErr
			}
			sess.UpsertRow(rowID, rowLabel, "", phasebox.Done)
			return nil
		},
		func(elapsed time.Duration) string {
			return fmt.Sprintf("%s — %s (%s)", resultVerb, cli.Config().ClusterName, elapsed.Round(time.Second))
		},
	)
	return cli, err
}
