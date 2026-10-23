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

// dockerImageClient is the subset of the moby docker client used to list and
// remove images during purge. Defined as an interface so the purge path can be
// unit-tested with a fake daemon instead of requiring a running one;
// *client.Client satisfies it.
type dockerImageClient interface {
	ImageList(ctx context.Context, options client.ImageListOptions) (client.ImageListResult, error)
	ImageRemove(
		ctx context.Context,
		imageID string,
		options client.ImageRemoveOptions,
	) (client.ImageRemoveResult, error)
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

// PruneCardinalImages removes the Docker images world built for THIS project, as
// well as old NATS images used by world start. This is intended to be called
// after containers have been purged.
func (c *Client) PruneCardinalImages(ctx context.Context) error {
	return pruneCardinalImages(ctx, c.client, c.logger, c.cfg.WorldToml.Project, c.builtImageNames())
}

// pruneCardinalImages implements PruneCardinalImages against the injectable
// dockerImageClient seam so the purge path can be unit-tested without a real
// daemon. owned is the set of image names this project's world.toml builds (see
// ownsBuiltImage). It is best-effort: a single image failing to remove is logged
// and skipped rather than aborting the whole purge.
func pruneCardinalImages(
	ctx context.Context,
	cli dockerImageClient,
	logger *slog.Logger,
	project string,
	owned map[string]struct{},
) error {
	// Every image world builds carries world-cli=cardinal. Listing on that alone and
	// filtering in Go, rather than ANDing a second label filter, is deliberate: the
	// project label was added later, so an image an older CLI built carries only the
	// first one and a two-label filter would silently stop reclaiming it.
	filters := make(client.Filters).
		Add("label", service.CardinalImageLabel+"="+service.CardinalImageValue)

	images, err := cli.ImageList(ctx, client.ImageListOptions{
		All:     true,
		Filters: filters,
	})
	if err != nil {
		return err
	}

	for _, img := range images.Items {
		if !ownsBuiltImage(img.Labels, project, owned) {
			continue
		}
		// Force: a rebuild keeps the tag, so the previous build is left untagged yet may
		// still be referenced, and the daemon then refuses an unforced delete and the image
		// leaks. The label cannot appear on an image world did not build, so forcing the
		// removal of one this project owns is safe and reclaims every tag in one call.
		_, removeErr := cli.ImageRemove(ctx, img.ID, client.ImageRemoveOptions{
			PruneChildren: true,
			Force:         true,
		})
		if removeErr != nil {
			// Best-effort cleanup: log and continue.
			logger.WarnContext(ctx, "failed to remove Cardinal image", "image", img.ID, "error", removeErr)
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

	allImages, err := cli.ImageList(ctx, client.ImageListOptions{All: true})
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

			_, removeErr := cli.ImageRemove(ctx, tag, client.ImageRemoveOptions{
				PruneChildren: true,
			})
			if removeErr != nil {
				logger.WarnContext(ctx, "failed to remove old world image", "tag", tag, "error", removeErr)
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

// builtImageNames is the set of image names this project's world.toml would build.
func (c *Client) builtImageNames() map[string]struct{} {
	names := CardinalBuildImageNames(
		c.ResolveServices(service.GetServices(c.cfg, service.CardinalShardsFirst)...),
	)
	owned := make(map[string]struct{}, len(names))
	for _, n := range names {
		owned[n] = struct{}{}
	}
	return owned
}

// ownsBuiltImage reports whether an image world built belongs to this project.
// The project label is authoritative when present. Images from a CLI older than
// that label fall back to the image name, which has always been project-scoped
// ("<project>-<id>-shard" / "<project>-<id>-service"). Matching on that prefix,
// not on membership in the current world.toml's build set, is what reclaims
// legacy images for shard/service IDs that were later removed or renamed from
// world.toml — the leak commit 792fac1d was written to stop — while still never
// matching another project's images: those carry that project's prefix.
//
// Project renames are a known limitation of the legacy fallback: a legacy image
// carries the old project's name, which the current world.toml no longer knows,
// so it cannot be matched safely. Images built by the current CLI carry the
// project label and are reclaimed across a rename via the label branch above.
func ownsBuiltImage(labels map[string]string, project string, owned map[string]struct{}) bool {
	if p, ok := labels[service.ProjectLabel]; ok {
		return p == project
	}
	name := labels[service.CardinalImageNameLabel]
	if _, ok := owned[name]; ok {
		return true
	}
	return legacyImageNameBelongsToProject(name, project)
}

// legacyImageNameBelongsToProject reports whether name is a world-built image
// name ("<project>-<id>-shard" / "<project>-<id>-service") scoped to project. It
// reclaims legacy images whose shard/service ID is no longer in world.toml
// without consulting the current build set. It does not match another project's
// images, which carry that project's prefix — modulo the inherent ambiguity of
// project names containing "-", which the project label exists to resolve and
// which legacy images lack.
func legacyImageNameBelongsToProject(name, project string) bool {
	if project == "" || name == "" {
		return false
	}
	if !strings.HasPrefix(name, project+"-") {
		return false
	}
	return strings.HasSuffix(name, "-"+service.RoleShard) ||
		strings.HasSuffix(name, "-"+service.RoleService)
}
