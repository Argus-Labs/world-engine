package docker

import (
	"archive/tar"
	"context"
	_ "embed"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/moby/go-archive"
	"github.com/moby/moby/api/types/build"
	"github.com/moby/moby/client"
	"github.com/moby/patternmatcher/ignorefile"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/rotisserie/eris"
	"golang.org/x/sync/errgroup"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
)

//go:embed cardinal.Dockerfile
var cardinalDockerfile []byte

// cardinalDockerfileName is the in-tar path under which the embedded Dockerfile
// is injected into every Cardinal build context. The leading underscore avoids
// collisions with anything a user might check in.
const cardinalDockerfileName = "__cardinal.Dockerfile"

// defaultBuildIgnores are always excluded from the Cardinal build context,
// regardless of the project's .dockerignore. They are appended *after* the
// user's .dockerignore patterns (see buildContextIgnorePatterns) so
// patternmatcher's last-wins semantics make them non-overridable: a consumer
// "!"-negation in .dockerignore (e.g. "!.git") cannot re-include them. The
// embedded cardinal.Dockerfile never invokes git, node_modules, dist, or any
// .exe, so re-including these paths would only bloat the build context.
var defaultBuildIgnores = []string{
	".git",
	".git/**",
	"node_modules",
	"**/node_modules",
	"dist",
	"**/dist",
	"**/*.exe",
}

// buildContextIgnorePatterns assembles the ExcludePatterns list for a Cardinal
// build context: the user's .dockerignore patterns first, then
// defaultBuildIgnores. patternmatcher evaluates ExcludePatterns with
// last-wins semantics, so placing the defaults last makes them non-overridable
// — a consumer "!"-negation (e.g. "!.git") cannot re-include a default-ignored
// path. User *additional* ignores, and "!"-negations targeting non-default
// paths, behave normally.
func buildContextIgnorePatterns(userIgnores []string) []string {
	return slices.Concat(userIgnores, defaultBuildIgnores)
}

// CardinalBuildImageNames returns the unique Cardinal image tags that would be
// built for services, preserving first-seen service order.
func CardinalBuildImageNames(services []service.Service) []string {
	names := make([]string, 0, len(services))
	seen := make(map[string]struct{}, len(services))
	for _, svc := range services {
		if !IsCardinalService(svc) {
			continue
		}
		if _, ok := seen[svc.Image]; ok {
			continue
		}
		seen[svc.Image] = struct{}{}
		names = append(names, svc.Image)
	}
	return names
}

// shardBuild is one image build plus every container that runs from it. Pool
// members share a shardBuild so the image is built once and reused.
type shardBuild struct {
	image       string
	serviceName string // representative service, used for build labels
	shardPath   string
	target      string
	containers  []string // all container names that use this image
}

// BuildCardinalImages builds Docker images for all Cardinal shard services
// using the dockerd-embedded BuildKit. Non-Cardinal services in the slice are
// silently skipped. The optional progress callback is called as each image
// build progresses. Passing nil means "no progress needed". The callback is
// serialized internally — callers do not need to synchronize.
//
// Progress fields used:
//   - Name:  the image reference being built (e.g. "my-shard:latest")
//   - State: StateBuilding while in progress, StateBuilt on completion
//   - Err:   non-nil when the individual image build fails (the function's
//     return value is the authoritative error; Err is for per-item UI updates)
func (c *Client) BuildCardinalImages(
	ctx context.Context,
	services []service.Service,
	progress func(Progress),
) error {
	progress = synchronized(progress)

	if c.cfg == nil {
		return eris.New("docker client missing config; cannot determine project root")
	}

	// Group by image tag so pooled replicas ("game", "game-2", ...) build once.
	builds := make(map[string]*shardBuild)
	var order []string
	for _, s := range services {
		if !IsCardinalService(s) {
			continue
		}

		shardPath, ok := s.BuildArgs["SHARD_PATH"]
		if !ok || strings.TrimSpace(shardPath) == "" {
			return eris.Errorf("cardinal service %s is missing SHARD_PATH build arg", s.Name)
		}

		b, exists := builds[s.Image]
		if !exists {
			b = &shardBuild{
				image:       s.Image,
				serviceName: s.Name,
				shardPath:   strings.TrimPrefix(shardPath, "./"),
				target:      s.BuildTarget,
			}
			builds[s.Image] = b
			order = append(order, s.Image)
		}
		b.containers = append(b.containers, s.Name)
	}

	if len(builds) == 0 {
		return nil
	}

	platform := ocispec.Platform{OS: "linux", Architecture: runtime.GOARCH}
	c.logger.InfoContext(ctx, "building Cardinal shard images with docker",
		"count", len(builds), "platform", platform)

	userIgnores, err := c.readDockerignore(ctx, c.cfg.RootDir)
	if err != nil {
		return eris.Wrap(err, "failed to read .dockerignore")
	}
	ignorePatterns := buildContextIgnorePatterns(userIgnores)

	g, gctx := errgroup.WithContext(ctx)
	for _, image := range order {
		build := builds[image]
		g.Go(func() error {
			notify(progress, Progress{Name: build.image, State: StateBuilding})

			err := c.buildSingleCardinalWithDocker(gctx, build, platform, ignorePatterns)
			if err != nil {
				notify(progress, Progress{Name: build.image, State: StateBuilding, Err: itemErr(gctx, err)})
				return err
			}

			notify(progress, Progress{Name: build.image, State: StateBuilt})
			return nil
		})
	}

	return g.Wait()
}

func (c *Client) buildSingleCardinalWithDocker(
	ctx context.Context,
	sb *shardBuild,
	platform ocispec.Platform,
	ignorePatterns []string,
) error {
	// Remove all containers on this image so they are recreated from the rebuild.
	for _, name := range sb.containers {
		if err := c.removeContainerKeepVolume(ctx, name); err != nil {
			c.logger.WarnContext(ctx,
				"failed to remove existing container before rebuild",
				"container", name, "error", err)
		}
	}

	c.logger.InfoContext(ctx, "building Cardinal shard with docker",
		"image", sb.image, "containers", sb.containers, "import", sb.shardPath)

	buildContext, err := tarWithEmbeddedDockerfile(
		c.cfg.RootDir,
		ignorePatterns,
		cardinalDockerfile,
		cardinalDockerfileName,
	)
	if err != nil {
		return eris.Wrap(err, "failed to build context tarball")
	}
	defer closeAndLog(ctx, c.logger, "build context", buildContext)

	target := sb.target
	if target == "" {
		target = "runtime"
	}

	shardPath := sb.shardPath
	// Forward GO_IMAGE from the GoBuilderImage constant so source builds use the
	// Go version the dependency pre-pull fetched, not cardinal.Dockerfile's older
	// ARG default — the constant is the single source of truth.
	goImage := service.GoBuilderImage
	opts := client.ImageBuildOptions{
		Tags:        []string{sb.image},
		Dockerfile:  cardinalDockerfileName,
		Target:      target,
		Platforms:   []ocispec.Platform{platform},
		Remove:      true,
		ForceRemove: true,
		Version:     build.BuilderBuildKit,
		BuildArgs: map[string]*string{
			"SHARD_PATH": &shardPath,
			"GO_IMAGE":   &goImage,
		},
		Labels: map[string]string{
			"world-cli":              "cardinal",
			"world-cli-service-name": sb.serviceName,
			"world-cli-image-name":   sb.image,
		},
	}

	resp, err := c.client.ImageBuild(ctx, buildContext, opts)
	if err != nil {
		return eris.Wrapf(err, "failed to start docker build for %s", sb.image)
	}
	defer closeAndLog(ctx, c.logger, "build response body", resp.Body)

	if err := drainBuildResponse(ctx, resp.Body, sb.image); err != nil {
		return eris.Wrapf(err, "failed to build Cardinal shard image %s", sb.image)
	}

	// Authoritative success check: the daemon must actually have a tag at the
	// expected name. This guards against silent successes if BuildKit ever
	// returns a clean event stream without producing the image.
	if _, err := c.client.ImageInspect(ctx, sb.image); err != nil {
		return eris.Wrapf(err, "build for %s returned success but image is not present", sb.image)
	}

	c.logger.InfoContext(ctx, "built and tagged shard image", "tag", sb.image)
	return nil
}

// readDockerignore reads patterns from <rootDir>/.dockerignore, returning nil
// if the file is absent.
func (c *Client) readDockerignore(ctx context.Context, rootDir string) ([]string, error) {
	f, err := os.Open(filepath.Join(rootDir, ".dockerignore"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer closeAndLog(ctx, c.logger, ".dockerignore", f)
	return ignorefile.ReadAll(f)
}

// tarWithEmbeddedDockerfile streams a tar archive of rootDir filtered by
// ignorePatterns, with dfContent injected at dfName. The injected entry takes
// precedence over any same-named entry in the source tree. The caller MUST
// fully read or close the returned ReadCloser; otherwise the producer
// goroutine leaks blocked on pw.Write.
func tarWithEmbeddedDockerfile(
	rootDir string,
	ignorePatterns []string,
	dfContent []byte,
	dfName string,
) (io.ReadCloser, error) {
	src, err := archive.TarWithOptions(rootDir, &archive.TarOptions{
		ExcludePatterns: ignorePatterns,
	})
	if err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()
	go func() {
		defer src.Close()
		tw := tar.NewWriter(pw)

		err := writeBuildContextTar(tw, src, dfContent, dfName)
		if cerr := tw.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		_ = pw.Close()
	}()

	return pr, nil
}

// writeBuildContextTar writes the embedded Dockerfile entry first, then copies
// every entry from src into tw, dropping any source entry named dfName.
func writeBuildContextTar(tw *tar.Writer, src io.Reader, dfContent []byte, dfName string) error {
	// Fixed epoch ModTime keeps the tar bit-stable across builds — the embedded
	// Dockerfile content never changes within a release, so its mtime shouldn't
	// either. Avoids a latent reproducibility hazard if BuildKit ever begins
	// hashing mtimes for cache keys.
	if err := tw.WriteHeader(&tar.Header{
		Name:    dfName,
		Size:    int64(len(dfContent)),
		Mode:    0o644,
		ModTime: time.Unix(0, 0).UTC(),
	}); err != nil {
		return err
	}
	if _, err := tw.Write(dfContent); err != nil {
		return err
	}

	tr := tar.NewReader(src)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Name == dfName {
			continue
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := io.Copy(tw, tr); err != nil {
			return err
		}
	}
}

// buildEvent captures the error fields from /build's JSON event stream
// (covering both the legacy builder and BuildKit shapes), plus BuildKit's
// trace, whose step output a build error quotes. Other progress is ignored —
// it's reported via the Progress callback.
type buildEvent struct {
	ID          string            `json:"id,omitempty"`
	Aux         json.RawMessage   `json:"aux,omitempty"`
	ErrorDetail *buildErrorDetail `json:"errorDetail,omitempty"`
	Error       string            `json:"error,omitempty"`
}

type buildErrorDetail struct {
	Message string `json:"message"`
}

func drainBuildResponse(ctx context.Context, body io.Reader, imageName string) error {
	var output buildOutput
	decoder := json.NewDecoder(body)
	for decoder.More() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var event buildEvent
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return eris.Wrapf(err, "decode build event for %s", imageName)
		}

		if event.ID == buildKitTraceID {
			var payload []byte
			if json.Unmarshal(event.Aux, &payload) == nil {
				output.add(payload)
			}
			continue
		}
		if event.ErrorDetail != nil && event.ErrorDetail.Message != "" {
			return eris.Errorf("build error for %s: %s%s", imageName, event.ErrorDetail.Message, output.tail())
		}
		if event.Error != "" {
			return eris.Errorf("build error for %s: %s%s", imageName, event.Error, output.tail())
		}
	}
	return nil
}

/////////////////////////////////////////////////////////////////////////////
/// Helper functions
/////////////////////////////////////////////////////////////////////////////

// closeAndLog runs c.Close and logs at warn level if it fails. Use as
// `defer closeAndLog(ctx, logger, "label", closer)` for io.Closers whose Close
// errors are non-fatal but worth surfacing for diagnostics.
func closeAndLog(ctx context.Context, logger *slog.Logger, label string, c io.Closer) {
	if err := c.Close(); err != nil {
		logger.WarnContext(ctx, "failed to close "+label, "error", err)
	}
}

// IsCardinalService returns true if the given service represents a Cardinal
// shard (identified by the Cardinal namespace label).
func IsCardinalService(s service.Service) bool {
	if len(s.Labels) == 0 {
		return false
	}
	_, ok := s.Labels[service.CardinalNamespaceLabel]
	return ok
}
