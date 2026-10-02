package root

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/rotisserie/eris"
	"golang.org/x/sync/errgroup"

	"github.com/argus-labs/world-engine/cli/internal/auth"
	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/pkg/cluster"
)

// Shared and branch environments use the regional GCP Gateway. A branch's
// path prefix selects its namespace-local operator.
func remoteOperatorEndpoint(env string) string {
	switch env {
	case "us-west1", "usw1":
		return "https://operator-usw1.argus.dev"
	case "us-west2", "usw2":
		return "https://operator-usw2.argus.dev"
	default:
		return fmt.Sprintf("https://operator-usw1.argus.dev/ephemeral/%s", env)
	}
}

// runRemoteLogs tails a deployed environment's shard logs. Platform pods
// (NATS, Postgres) are not available: the operator only reaches its own
// namespace's shards, and the local path to them needs a kubeconfig.
func runRemoteLogs(ctx context.Context, env string, shards []string, tail int32, previous bool) error {
	env = strings.TrimSpace(env)
	if env == "" {
		return eris.New("--env is empty")
	}

	cli := cluster.NewClient(cluster.Config{
		OperatorEndpoint: remoteOperatorEndpoint(env),
		TokenSource:      auth.New(),
	})

	printer.Infof("Tailing %s\n", env)

	lines := make(chan cluster.LogLine, 256)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return cli.StreamShardLogs(gctx, cluster.LogsOpts{
			ShardIDs:  shards,
			TailLines: tail,
			Previous:  previous,
		}, lines)
	})
	g.Go(func() error {
		for line := range lines {
			_, _ = fmt.Fprintf(os.Stdout, "%-18s %s\n", line.InstanceName, line.Line)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return eris.Wrapf(err, "streaming logs for %s", env)
	}
	return nil
}
