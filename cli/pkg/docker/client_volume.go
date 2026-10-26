package docker

import (
	"context"
	"errors"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
	"github.com/rotisserie/eris"
)

// dockerVolumeClient is the moby subset the volume sweep uses, so it can be unit-tested with a fake.
type dockerVolumeClient interface {
	VolumeRemove(
		ctx context.Context,
		volumeID string,
		options client.VolumeRemoveOptions,
	) (client.VolumeRemoveResult, error)
}

// RemoveVolumes removes the named volumes. Every name is attempted, not-found
// is skipped, and the remaining errors are joined. Force only suppresses
// driver errors: a volume still referenced by a container is reported as in use.
func (c *Client) RemoveVolumes(ctx context.Context, names []string) error {
	return removeVolumes(ctx, c.client, names)
}

func removeVolumes(ctx context.Context, cli dockerVolumeClient, names []string) error {
	var errs []error
	for _, name := range names {
		if name == "" {
			continue
		}
		if _, err := cli.VolumeRemove(ctx, name, client.VolumeRemoveOptions{Force: true}); err != nil &&
			!cerrdefs.IsNotFound(err) {
			errs = append(errs, eris.Wrapf(err, "remove volume %s", name))
		}
	}
	return errors.Join(errs...)
}
