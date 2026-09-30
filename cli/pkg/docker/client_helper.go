package docker

import (
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
)

func WithClient(
	projectDir string,
	debug bool,
	opts *ClientOptions,
	fn func(cfg *service.Config, client *Client) error,
) error {
	cfg, err := NewClientConfig(projectDir, debug)
	if err != nil {
		return err
	}

	client, err := NewClient(cfg, opts)
	if err != nil {
		return eris.Wrap(err, "failed to create docker client")
	}
	defer func() {
		if err := client.Close(); err != nil {
			client.logger.Error("failed to close docker client", "error", err)
		}
	}()

	return fn(cfg, client)
}
