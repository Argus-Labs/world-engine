package docker

import (
	"context"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
	"github.com/rotisserie/eris"
)

// EnsureNetwork creates a bridge network if it does not exist. Idempotent.
func (c *Client) EnsureNetwork(ctx context.Context, name string) error {
	_, err := c.client.NetworkInspect(ctx, name, client.NetworkInspectOptions{})
	if err == nil {
		return nil
	}
	if !cerrdefs.IsNotFound(err) {
		return eris.Wrapf(err, "inspect network %s", name)
	}
	if _, err := c.client.NetworkCreate(ctx, name, client.NetworkCreateOptions{Driver: "bridge"}); err != nil {
		return eris.Wrapf(err, "create network %s", name)
	}
	return nil
}

// RemoveNetwork deletes a network; missing is not an error.
func (c *Client) RemoveNetwork(ctx context.Context, name string) error {
	_, err := c.client.NetworkRemove(ctx, name, client.NetworkRemoveOptions{})
	if err != nil && !cerrdefs.IsNotFound(err) {
		return eris.Wrapf(err, "remove network %s", name)
	}
	return nil
}
