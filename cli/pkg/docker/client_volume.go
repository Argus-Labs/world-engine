package docker

import (
	"context"
	"errors"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
)

// dockerVolumeClient is the moby subset the volume sweep uses, so it can be unit-tested with a fake.
type dockerVolumeClient interface {
	VolumeInspect(
		ctx context.Context,
		volumeID string,
		options client.VolumeInspectOptions,
	) (client.VolumeInspectResult, error)
	VolumeRemove(
		ctx context.Context,
		volumeID string,
		options client.VolumeRemoveOptions,
	) (client.VolumeRemoveResult, error)
}

// RemoveVolumes removes the named volumes. Every name is attempted, not-found
// is skipped, and the remaining errors are joined. Force only suppresses
// driver errors: a volume still referenced by a container is reported as in use.
// A volume labelled with another project is kept and reported: service volume
// names join project and id with hyphens, so project "demo" service "game-api"
// and project "demo-game" service "api" share a name.
func (c *Client) RemoveVolumes(ctx context.Context, names []string) error {
	return removeVolumes(ctx, c.client, c.cfg.WorldToml.Project, names)
}

func removeVolumes(ctx context.Context, cli dockerVolumeClient, project string, names []string) error {
	var errs []error

	for _, name := range names {
		if name == "" {
			continue
		}

		if err := removeVolume(ctx, cli, project, name); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// removeVolume removes one volume unless it is labelled with another project.
// Unlabelled volumes, from a CLI that predates the label, are still removed.
func removeVolume(ctx context.Context, cli dockerVolumeClient, project, name string) error {
	res, err := cli.VolumeInspect(ctx, name, client.VolumeInspectOptions{})
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return nil
		}

		return eris.Wrapf(err, "inspect volume %s", name)
	}

	if owner, ok := res.Volume.Labels[service.ProjectLabel]; ok && owner != project {
		return eris.Errorf(
			"volume %s belongs to project %q, not %q; rename a service in world.toml so the names differ",
			name,
			owner,
			project,
		)
	}

	if _, err := cli.VolumeRemove(ctx, name, client.VolumeRemoveOptions{Force: true}); err != nil &&
		!cerrdefs.IsNotFound(err) {
		return eris.Wrapf(err, "remove volume %s", name)
	}

	return nil
}
