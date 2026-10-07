package docker

import (
	"context"
	"log/slog"
	"strings"

	"github.com/moby/moby/client"
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	"github.com/argus-labs/world-engine/cli/pkg/version"
)

// ClientOptions holds optional configuration for a Client.
// Pass nil to NewClient for default behaviour (silent logging).
type ClientOptions struct {
	Logger *slog.Logger
}

type Client struct {
	client *client.Client
	cfg    *service.Config
	logger *slog.Logger
}

func NewClient(cfg *service.Config, opts *ClientOptions) (*Client, error) {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return nil, eris.Wrap(err, "Failed to create docker client")
	}

	logger := slog.New(slog.DiscardHandler)
	if opts != nil && opts.Logger != nil {
		logger = opts.Logger
	}

	return &Client{
		client: cli,
		cfg:    cfg,
		logger: logger,
	}, nil
}

func (c *Client) Close() error {
	return c.client.Close()
}

// ResolveServices resolves service builders into concrete service definitions.
func (c *Client) ResolveServices(builders ...service.Builder) []service.Service {
	services := make([]service.Service, 0, len(builders))
	for _, sb := range builders {
		services = append(services, sb(c.cfg))
	}
	return services
}

// PruneCardinalImages removes Docker images that were built by world-cli for
// Cardinal (identified via the world-cli=cardinal image label), as well as old
// NATS images used by world start. This is intended to be called after
// containers have been purged.
func (c *Client) PruneCardinalImages(ctx context.Context) error {
	// First, remove any images that were built by world-cli for Cardinal.
	filters := make(client.Filters).Add("label", service.CardinalImageLabel+"="+service.CardinalImageValue)

	images, err := c.client.ImageList(ctx, client.ImageListOptions{
		All:     true,
		Filters: filters,
	})
	if err != nil {
		return err
	}

	for _, img := range images.Items {
		_, removeErr := c.client.ImageRemove(ctx, img.ID, client.ImageRemoveOptions{
			PruneChildren: true,
		})
		if removeErr != nil {
			// Best-effort cleanup: log and continue.
			c.logger.WarnContext(ctx, "failed to remove Cardinal image", "image", img.ID, "error", removeErr)
		}
	}

	// Then, best-effort remove old NATS image versions, keeping only the current
	// version in use by world-cli.
	currentImages := map[string]struct{}{
		service.DefaultNatsImage + version.Nats: {},
	}

	worldImagePrefixes := []string{
		service.DefaultNatsImage,
	}

	allImages, err := c.client.ImageList(ctx, client.ImageListOptions{All: true})
	if err != nil {
		return err
	}

	for _, img := range allImages.Items {
		for _, tag := range img.RepoTags {
			// Skip non-world images quickly.
			if !hasWorldImagePrefix(tag, worldImagePrefixes) {
				continue
			}
			// Keep the current image versions.
			if _, keep := currentImages[tag]; keep {
				continue
			}

			_, removeErr := c.client.ImageRemove(ctx, tag, client.ImageRemoveOptions{
				PruneChildren: true,
			})
			if removeErr != nil {
				c.logger.WarnContext(ctx, "failed to remove old world image", "tag", tag, "error", removeErr)
			}
		}
	}

	return nil
}

// hasWorldImagePrefix reports whether tag starts with any of the given prefixes.
func hasWorldImagePrefix(tag string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(tag, prefix) {
			return true
		}
	}
	return false
}
