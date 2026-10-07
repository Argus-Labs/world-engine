package debugger

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/internal/debug"
	"github.com/argus-labs/world-engine/cli/internal/printer"
	"github.com/argus-labs/world-engine/cli/pkg/local"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// debugTarget is one shard instance and the URL its DebugService answers on.
type debugTarget struct {
	instanceID string
	url        string
}

// resolveTargets pairs selected instances with their DebugService URLs.
func resolveTargets(dir string, instanceIDs []string) ([]debugTarget, error) {
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, eris.Wrap(err, "failed to get current directory")
		}
		dir = wd
	}

	cfg, err := worldtoml.LoadFile(filepath.Join(dir, worldtoml.FileName))
	if err != nil {
		return nil, eris.Wrapf(err, "failed to load %s", worldtoml.FileName)
	}

	instances, err := cfg.ResolveInstanceIDs(instanceIDs)
	if err != nil {
		return nil, err
	}

	targets := make([]debugTarget, 0, len(instances))
	for _, instance := range instances {
		targets = append(targets, debugTarget{
			instanceID: instance.InstanceID,
			url:        local.ShardAPIURL(cfg.Organization, cfg.Project, instance.InstanceID),
		})
	}
	return targets, nil
}

// runDebugAction runs action in parallel and reports results in target order.
func runDebugAction(ctx context.Context, instanceIDs []string, verb string, action debug.Action) error {
	targets, err := resolveTargets("", instanceIDs)
	if err != nil {
		return err
	}

	msgs := make([]string, len(targets))
	errs := make([]error, len(targets))

	var wg sync.WaitGroup
	for i, target := range targets {
		wg.Go(func() {
			msgs[i], errs[i] = action(ctx, debug.NewClient(target.url), target.instanceID)
		})
	}
	wg.Wait()

	failures := 0
	for i, target := range targets {
		if errs[i] != nil {
			failures++
			printer.Errorf("Failed to %s instance %q (%s): %v\n",
				verb, target.instanceID, target.url, errs[i])
			continue
		}
		printer.Successf("%s\n", msgs[i])
	}
	if failures > 0 {
		return eris.Errorf("%d of %d instance(s) failed to %s", failures, len(targets), verb)
	}
	return nil
}
