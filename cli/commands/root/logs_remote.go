package root

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/rotisserie/eris"
	"golang.org/x/sync/errgroup"

	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/pkg/cluster"
	"github.com/argus-labs/world-engine/cli/pkg/worldstatus"
)

// runRemoteLogs tails shard logs on a kubeconfig context (the hosted dev server, an
// ephemeral environment). Access is whatever the developer's kubeconfig grants.
func runRemoteLogs(
	ctx context.Context,
	kubeContext, namespace, project string,
	shards []string,
	tail int32,
	previous bool,
) error {
	kubeContext = strings.TrimSpace(kubeContext)
	if kubeContext == "" {
		return eris.New("--context is empty")
	}
	if namespace == "" {
		namespace = cluster.ProjectNamespace(project)
	}

	cli := cluster.NewClient(cluster.Config{Context: kubeContext})

	printer.Infof("Tailing %s/%s\n", kubeContext, namespace)

	lines := make(chan worldstatus.LogLine, 256)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return cli.StreamShardLogs(gctx, worldstatus.LogsOpts{
			Namespace: namespace,
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
		return eris.Wrapf(err, "streaming logs for %s/%s", kubeContext, namespace)
	}
	return nil
}
