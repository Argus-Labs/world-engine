package docker

import (
	"context"
	"encoding/base64"
	"io"
	"strings"

	"github.com/goccy/go-json"
	"github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/rotisserie/eris"
	"golang.org/x/sync/errgroup"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
)

type pullEvent struct {
	Status         string           `json:"status"`
	Error          string           `json:"error"`
	ErrorDetail    *pullErrorDetail `json:"errorDetail,omitempty"`
	ProgressDetail *pullProgress    `json:"progressDetail,omitempty"`
}

type pullErrorDetail struct {
	Message string `json:"message"`
}

type pullProgress struct {
	Current int64 `json:"current"`
	Total   int64 `json:"total"`
}

// PullImages pulls all required images for the given services concurrently.
// The optional progress callback is called as each image pull progresses.
// Passing nil means "no progress needed". The callback is serialized
// internally — callers do not need to synchronize.
//
// Progress fields used:
//   - Name:    the image reference being pulled (e.g. "docker.io/library/golang:1.25")
//   - State:   StatePulling while in progress, StatePulled on completion
//   - Current: download percentage (0–100), only meaningful when State == StatePulling
//   - Total:   always 100
//   - Err:     non-nil when the individual image pull fails (the function's
//     return value is the authoritative error; Err is for per-item UI updates)
func (c *Client) PullImages(ctx context.Context, services []service.Service, progress func(Progress)) error {
	progress = synchronized(progress)
	// Filter the images that need to be pulled
	images := make(map[string]ocispec.Platform)
	c.filterImages(ctx, images, services...)

	if len(images) == 0 {
		return nil
	}

	c.logger.InfoContext(ctx, "starting to pull images", "count", len(images))

	g, gctx := errgroup.WithContext(ctx)

	for imageName, platform := range images {
		g.Go(func() error {
			notify(progress, Progress{Name: imageName, State: StatePulling, Current: 0, Total: 100})

			err := c.pullSingleImage(gctx, imageName, platform, func(percent int64) {
				notify(progress, Progress{Name: imageName, State: StatePulling, Current: percent, Total: 100})
			})
			if err != nil {
				notify(progress, Progress{Name: imageName, State: StatePulling, Err: err})
				return eris.Wrapf(err, "error pulling image %s", imageName)
			}

			notify(progress, Progress{Name: imageName, State: StatePulled, Current: 100, Total: 100})
			return nil
		})
	}

	return g.Wait()
}

// PullImageRefs pulls each of refs (bare image refs, e.g.
// "docker.io/rancher/k3s:v1.31.5-k3s1") concurrently, deduping and
// skipping cached images (same ":latest" policy as PullImages). The
// plain-ref counterpart to PullImages for callers with an explicit image
// list, like cluster.Client.RequiredBootstrapImages. Progress fields match
// PullImages'.
func (c *Client) PullImageRefs(ctx context.Context, refs []string, progress func(Progress)) error {
	progress = synchronized(progress)

	unique := make(map[string]struct{}, len(refs))
	g, gctx := errgroup.WithContext(ctx)
	for _, ref := range refs {
		if _, seen := unique[ref]; seen {
			continue
		}
		unique[ref] = struct{}{}

		g.Go(func() error {
			if !c.needsPull(gctx, ref) {
				return nil
			}

			notify(progress, Progress{Name: ref, State: StatePulling, Current: 0, Total: 100})

			err := c.pullSingleImage(gctx, ref, ocispec.Platform{}, func(percent int64) {
				notify(progress, Progress{Name: ref, State: StatePulling, Current: percent, Total: 100})
			})
			if err != nil {
				notify(progress, Progress{Name: ref, State: StatePulling, Err: err})
				return eris.Wrapf(err, "error pulling image %s", ref)
			}

			notify(progress, Progress{Name: ref, State: StatePulled, Current: 100, Total: 100})
			return nil
		})
	}
	return g.Wait()
}

// RefsToPull is PullImageRefs' dry run: returns the refs it would actually
// pull (deduplicated), without pulling. Empty means callers can skip
// opening progress UI.
func (c *Client) RefsToPull(ctx context.Context, refs []string) []string {
	seen := make(map[string]struct{}, len(refs))
	var out []string
	for _, ref := range refs {
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		if c.needsPull(ctx, ref) {
			out = append(out, ref)
		}
	}
	return out
}

// needsPull reports whether ref isn't already present locally, or is but is
// ":latest"-tagged (always re-checked for a newer version, matching
// filterImages' policy for services).
func (c *Client) needsPull(ctx context.Context, ref string) bool {
	if strings.HasSuffix(ref, ":latest") {
		return true
	}
	_, err := c.client.ImageInspect(ctx, ref)
	return err != nil
}

// ImagesToPull is PullImages' dry run: returns the image names it would
// actually pull for services (cached/non-":latest" images excluded),
// without pulling. Empty means callers can skip opening progress UI.
func (c *Client) ImagesToPull(ctx context.Context, services []service.Service) []string {
	images := make(map[string]ocispec.Platform)
	c.filterImages(ctx, images, services...)
	names := make([]string, 0, len(images))
	for name := range images {
		names = append(names, name)
	}
	return names
}

// filterImages filters the images that need to be pulled.
// Remove duplicates.
// Remove images that are already pulled.
// Remove images that need to be built.
func (c *Client) filterImages(ctx context.Context, images map[string]ocispec.Platform, services ...service.Service) {
	for _, svc := range services {
		alwaysPull := strings.HasSuffix(svc.Image, ":latest")

		// check if the image exists
		_, err := c.client.ImageInspect(ctx, svc.Image)
		if err == nil && !alwaysPull {
			// Image already exists and no force policy, skip pulling
			c.logger.DebugContext(ctx, "image already exists, skipping pull", "image", svc.Image)
			// Recursively check dependencies even if image is present
			if svc.Dependencies != nil {
				c.logger.DebugContext(ctx, "checking dependencies", "service", svc.Name)
				c.filterImages(ctx, images, svc.Dependencies...)
			}
			continue
		}

		if alwaysPull {
			c.logger.DebugContext(ctx, "force pulling latest image", "image", svc.Image)
		}

		// check if the image needs to be built
		// if the service has a Dockerfile or BuildTarget, it needs to be built
		if svc.Dockerfile == "" && svc.BuildTarget == "" {
			// Image does not exist and does not need to be built
			// Add the image to the list of images to pull
			if hasPlatform(svc.Platform) {
				images[svc.Image] = svc.Platform
				c.logger.DebugContext(ctx, "added image to pull list", "image", svc.Image, "platform", svc.Platform)
			} else {
				images[svc.Image] = ocispec.Platform{}
				c.logger.DebugContext(ctx, "added image to pull list", "image", svc.Image)
			}
		} else {
			c.logger.DebugContext(ctx, "image will be built, skipping pull", "image", svc.Image)
		}

		// Recursively check dependencies
		if svc.Dependencies != nil {
			c.logger.DebugContext(ctx, "checking dependencies", "service", svc.Name)
			c.filterImages(ctx, images, svc.Dependencies...)
		}
	}
}

// pullSingleImage pulls a single Docker image and reports progress via onPercent.
func (c *Client) pullSingleImage(
	ctx context.Context,
	imageName string,
	platform ocispec.Platform,
	onPercent func(int64),
) error {
	c.logger.InfoContext(ctx, "starting pull for image", "image", imageName)

	// Prepare pull options
	pullOptions, err := c.preparePullOptions(platform, nil)
	if err != nil {
		return err
	}

	// Start pulling the image
	responseBody, err := c.client.ImagePull(ctx, imageName, pullOptions)
	if err != nil {
		return eris.Wrapf(err, "error pulling image %s", imageName)
	}
	defer responseBody.Close()

	// Process pull events and update progress
	if err := c.processPullEvents(ctx, responseBody, imageName, onPercent); err != nil {
		return err
	}

	c.logger.InfoContext(ctx, "completed pull for image", "image", imageName)
	return nil
}

// preparePullOptions creates pull options with authentication if needed.
func (c *Client) preparePullOptions(
	platform ocispec.Platform,
	authConfig *registry.AuthConfig,
) (client.ImagePullOptions, error) {
	pullOptions := client.ImagePullOptions{}

	if hasPlatform(platform) {
		pullOptions.Platforms = append(pullOptions.Platforms, platform)
	}

	if authConfig == nil {
		return pullOptions, nil
	}

	authBytes, err := json.Marshal(*authConfig)
	if err != nil {
		return pullOptions, eris.Wrap(err, "failed to encode auth")
	}

	pullOptions.RegistryAuth = base64.URLEncoding.EncodeToString(authBytes)
	return pullOptions, nil
}

func hasPlatform(platform ocispec.Platform) bool {
	return platform.OS != "" || platform.Architecture != "" || platform.Variant != ""
}

// processPullEvents processes Docker pull events and reports progress via onPercent.
func (c *Client) processPullEvents(
	ctx context.Context,
	responseBody io.ReadCloser,
	imageName string,
	onPercent func(int64),
) error {
	decoder := json.NewDecoder(responseBody)
	var current int64
	var event pullEvent

	for decoder.More() {
		select {
		case <-ctx.Done():
			c.logger.InfoContext(ctx, "image pull canceled", "image", imageName)
			return ctx.Err()
		default:
			if err := decoder.Decode(&event); err != nil {
				return eris.Errorf("error decoding event for %s: %v", imageName, err)
			}

			if err := checkPullEventError(event, imageName); err != nil {
				return err
			}

			if percent, ok := extractPullPercent(event); ok && percent > current {
				current = percent
				if onPercent != nil {
					onPercent(current)
				}
				c.logger.DebugContext(ctx, "pull progress", "image", imageName, "percent", current)
			}
		}
	}
	return nil
}

// checkPullEventError checks for error fields in a Docker pull event.
func checkPullEventError(event pullEvent, imageName string) error {
	if event.ErrorDetail != nil && event.ErrorDetail.Message != "" {
		return eris.Errorf("pull error for image %s: %s", imageName, event.ErrorDetail.Message)
	}

	if event.Error != "" {
		return eris.Errorf("pull error for image %s: %s", imageName, event.Error)
	}
	return nil
}

// extractPullPercent extracts the completion percentage from a Docker pull event.
func extractPullPercent(event pullEvent) (int64, bool) {
	if event.ProgressDetail == nil || event.ProgressDetail.Total <= 0 {
		return 0, false
	}

	percent := event.ProgressDetail.Current * 100 / event.ProgressDetail.Total
	return percent, true
}
