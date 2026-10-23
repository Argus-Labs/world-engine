package docker

import (
	"context"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
	"github.com/rotisserie/eris"
)

// dockerVolumeClient is the subset of the moby docker client used to sweep a
// project's named volumes during purge. Defined as an interface so the volume
// sweep can be unit-tested with a fake daemon instead of requiring a running
// one; *client.Client satisfies it.
type dockerVolumeClient interface {
	VolumeRemove(
		ctx context.Context,
		volumeID string,
		options client.VolumeRemoveOptions,
	) (client.VolumeRemoveResult, error)
}

// RemoveVolumes removes the named volumes; a missing volume is not an error.
// Purge calls this after RemoveContainers to reclaim a volume whose container
// was already removed (a failed recreate, `docker rm`, or `docker container
// prune`): such a volume is invisible to the mount walk inside
// RemoveContainers, so without this sweep purge would leave it on disk and the
// next `world start` would bind the same deterministic name and resurrect the
// old state.
//
// Every volume is attempted regardless of earlier failures so one stuck volume
// does not strand the others; Force removes a volume still referenced by a
// stopped container, and not-found is skipped so an already-wiped volume (or
// one purge never created) is not a failure. A non-not-found error is returned
// so the caller does not report "(all state wiped)" for a purge that leaked.
func (c *Client) RemoveVolumes(ctx context.Context, names []string) error {
	return removeVolumes(ctx, c.client, names)
}

// removeVolumes is split from RemoveVolumes so the purge path can be unit-tested
// with a fake daemon instead of requiring a running one.
func removeVolumes(ctx context.Context, cli dockerVolumeClient, names []string) error {
	var firstErr error
	for _, name := range names {
		if name == "" {
			continue
		}
		if _, err := cli.VolumeRemove(ctx, name, client.VolumeRemoveOptions{Force: true}); err != nil &&
			!cerrdefs.IsNotFound(err) {
			if firstErr == nil {
				firstErr = eris.Wrapf(err, "remove volume %s", name)
			}
		}
	}
	return firstErr
}
