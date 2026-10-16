package docker

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/version"
)

// fakeImageClient is a stand-in dockerImageClient for unit-testing the purge
// path without a running daemon. It serves a fixed population of images,
// optionally filtered by the "label" term callers pass to ImageList, and
// records every ImageRemove call (including the returned error).
//
// By default ImageRemove mimics the real daemon's tag semantics: an image
// referenced by 2+ repo tags cannot be removed by ID without Force ("image is
// referenced in multiple repositories"), which is the exact behavior the
// purge bug hinges on. Removal by repo tag targets a single tag and always
// succeeds regardless of Force. Either behavior can be overridden with
// removeFn; ImageList can be overridden with listFn.
type fakeImageClient struct {
	mu       sync.Mutex
	images   []image.Summary
	removed  []fakeRemoveCall
	listFn   func(opts client.ImageListOptions) (client.ImageListResult, error)
	removeFn func(imageRef string, opts client.ImageRemoveOptions) error
}

type fakeRemoveCall struct {
	imageRef string
	force    bool
	prune    bool
	err      error
}

func (f *fakeImageClient) ImageList(_ context.Context, opts client.ImageListOptions) (client.ImageListResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listFn != nil {
		return f.listFn(opts)
	}
	out := make([]image.Summary, 0, len(f.images))
	for _, img := range f.images {
		if !fakeImageMatchesFilters(img, opts.Filters) {
			continue
		}
		out = append(out, img)
	}
	return client.ImageListResult{Items: out}, nil
}

func (f *fakeImageClient) ImageRemove(
	_ context.Context,
	imageRef string,
	opts client.ImageRemoveOptions,
) (client.ImageRemoveResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var rerr error
	switch {
	case f.removeFn != nil:
		rerr = f.removeFn(imageRef, opts)
	case !opts.Force && strings.HasPrefix(imageRef, "sha256:") && fakeRepoTagCount(f.images, imageRef) >= 2:
		// Daemon-accurate: a multi-tagged image removed by ID without Force is
		// rejected with the conflict the purge bug centers on.
		rerr = errors.New(
			"conflict: unable to delete " + imageRef +
				" (must be forced) - image is referenced in multiple repositories",
		)
	}

	f.removed = append(f.removed, fakeRemoveCall{
		imageRef: imageRef,
		force:    opts.Force,
		prune:    opts.PruneChildren,
		err:      rerr,
	})
	return client.ImageRemoveResult{}, rerr
}

func fakeImageMatchesFilters(img image.Summary, filters client.Filters) bool {
	if len(filters) == 0 {
		return true
	}
	for term, values := range filters {
		if term != "label" {
			continue
		}
		matched := false
		for v := range values {
			if fakeLabelMatches(img.Labels, v) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func fakeLabelMatches(labels map[string]string, want string) bool {
	if k, v, ok := strings.Cut(want, "="); ok {
		return labels[k] == v
	}
	_, ok := labels[want]
	return ok
}

func fakeRepoTagCount(images []image.Summary, id string) int {
	for _, img := range images {
		if img.ID == id {
			return len(img.RepoTags)
		}
	}
	return 0
}

// cardinalImage builds a world-cli=cardinal-labeled image with the given image
// ID and repo tags — modeling what `world reload` leaves on the host.
func cardinalImage(id string, tags ...string) image.Summary {
	return image.Summary{
		ID:       id,
		RepoTags: tags,
		Labels:   map[string]string{"world-cli": "cardinal"},
	}
}

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// TestPruneCardinalImages_ForcesRemovalOfEveryLabeledImage is the primary
// regression guard for the fix: every Cardinal-labeled image must be removed
// with Force=true. On the buggy (unforced) code path the fake daemon rejects
// the multi-tagged images, so this assertion fails there.
func TestPruneCardinalImages_ForcesRemovalOfEveryLabeledImage(t *testing.T) {
	t.Parallel()

	fake := &fakeImageClient{
		images: []image.Summary{
			// Last build of a shard ID later removed from world.toml: its source
			// tag plus the per-reload registry tag — always 2 refs.
			cardinalImage(
				"sha256:stalelastbuild",
				"rampage-stale-shard:latest",
				"k3d-testreg.localhost:5000/rampage/stale:local-1780199123",
			),
			// Current-ID last build, likewise multi-tagged.
			cardinalImage(
				"sha256:currentlastbuild",
				"rampage-gameplay-shard:latest",
				"k3d-testreg.localhost:5000/rampage/gameplay:local-1780200000",
			),
		},
	}

	require.NoError(t, pruneCardinalImages(context.Background(), fake, discardLogger()))

	require.Len(t, fake.removed, 2, "both labeled images must be removed")
	for _, call := range fake.removed {
		assert.True(t, call.force, "labeled image %q must be removed with Force (the fix)", call.imageRef)
		assert.True(t, call.prune, "PruneChildren must remain set for %q", call.imageRef)
		assert.NoError(t, call.err, "multi-tagged labeled image %q must actually be removed", call.imageRef)
	}
}

// TestPruneCardinalImages_RemovesStaleMultiTaggedID pins the exact leak
// scenario from the bug report: a shard/service ID removed from world.toml
// whose last build carries 2+ tags would survive unforced pass-1 and is
// invisible to pass-2 (current-world.toml enumeration). With Force it is
// removed — the fake daemon confirms success rather than the
// "must be forced" rejection.
func TestPruneCardinalImages_RemovesStaleMultiTaggedID(t *testing.T) {
	t.Parallel()

	const staleID = "sha256:stalelastbuild"
	fake := &fakeImageClient{
		images: []image.Summary{
			cardinalImage(
				staleID,
				"rampage-stale-shard:latest",
				"k3d-testreg.localhost:5000/rampage/stale:local-1780199123",
			),
		},
	}

	require.NoError(t, pruneCardinalImages(context.Background(), fake, discardLogger()))
	require.Len(t, fake.removed, 1)
	assert.Equal(t, staleID, fake.removed[0].imageRef)
	assert.True(t, fake.removed[0].force, "stale multi-tagged image requires Force")
	assert.NoError(t, fake.removed[0].err, "stale image must be removed, not leaked")
}

// TestPruneCardinalImages_RemovesSingleTaggedLabeledImage confirms there is no
// regression for the common case (older builds that lost their source tag and
// carry only the per-reload registry ref) — these were already removable
// unforced and must remain removable with the forced path.
func TestPruneCardinalImages_RemovesSingleTaggedLabeledImage(t *testing.T) {
	t.Parallel()

	fake := &fakeImageClient{
		images: []image.Summary{
			cardinalImage(
				"sha256:oldoldbuild",
				"k3d-testreg.localhost:5000/rampage/gameplay:local-1780100000",
			),
		},
	}

	require.NoError(t, pruneCardinalImages(context.Background(), fake, discardLogger()))
	require.Len(t, fake.removed, 1)
	assert.NoError(t, fake.removed[0].err, "single-tagged labeled image must be removed")
	assert.True(t, fake.removed[0].force)
}

// TestPruneCardinalImages_BestEffortLogsAndContinues verifies a single removal
// failure does not abort the purge: every labeled image is still attempted and
// the NATS pass still runs (no error returned).
func TestPruneCardinalImages_BestEffortLogsAndContinues(t *testing.T) {
	t.Parallel()

	fake := &fakeImageClient{
		images: []image.Summary{
			cardinalImage("sha256:a", "rampage-a-shard:latest"),
			cardinalImage("sha256:b", "rampage-b-shard:latest"),
		},
		removeFn: func(imageRef string, _ client.ImageRemoveOptions) error {
			if imageRef == "sha256:a" {
				return errors.New("daemon unavailable")
			}
			return nil
		},
	}

	require.NoError(t, pruneCardinalImages(context.Background(), fake, discardLogger()))
	require.Len(t, fake.removed, 2, "removal must be attempted for every labeled image")
	require.Error(t, fake.removed[0].err, "the failing image's error must be recorded")
	assert.NoError(t, fake.removed[1].err, "the healthy image must still be removed")
}

// TestPruneCardinalImages_NoLabeledImagesIsNoOp confirms an empty Cardinal
// pass does not error and attempts no removals.
func TestPruneCardinalImages_NoLabeledImagesIsNoOp(t *testing.T) {
	t.Parallel()

	fake := &fakeImageClient{images: nil}

	require.NoError(t, pruneCardinalImages(context.Background(), fake, discardLogger()))
	assert.Empty(t, fake.removed, "no Cardinal-labeled images means no removal attempts")
}

// TestPruneCardinalImages_NATSKeepsCurrentAndPrunesOld verifies the second
// (NATS) pass is unchanged by the fix: only non-current NATS tags are removed,
// the current version is kept, and unrelated images are untouched. NATS
// removal is by tag so Force is not asserted there.
func TestPruneCardinalImages_NATSKeepsCurrentAndPrunesOld(t *testing.T) {
	t.Parallel()

	current := "nats:" + version.Nats
	fake := &fakeImageClient{
		images: []image.Summary{
			{ID: "sha256:natscurrent", RepoTags: []string{current}},
			{ID: "sha256:natsold", RepoTags: []string{"nats:2.10.0"}},
			{ID: "sha256:ubuntu", RepoTags: []string{"ubuntu:latest"}},
		},
	}

	require.NoError(t, pruneCardinalImages(context.Background(), fake, discardLogger()))

	removed := make([]string, 0, len(fake.removed))
	for _, c := range fake.removed {
		removed = append(removed, c.imageRef)
	}
	assert.Equal(t, []string{"nats:2.10.0"}, removed, "only the non-current NATS tag must be removed")
}

// TestPruneCardinalImages_CardinalListErrorPropagates verifies an ImageList
// failure in the Cardinal pass is returned rather than swallowed.
func TestPruneCardinalImages_CardinalListErrorPropagates(t *testing.T) {
	t.Parallel()

	fake := &fakeImageClient{
		listFn: func(opts client.ImageListOptions) (client.ImageListResult, error) {
			if _, ok := opts.Filters["label"]; ok {
				return client.ImageListResult{}, errors.New("cardinal list failed")
			}
			return client.ImageListResult{}, nil
		},
	}

	err := pruneCardinalImages(context.Background(), fake, discardLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cardinal list failed")
}

// TestPruneCardinalImages_NATSListErrorPropagates verifies an ImageList
// failure in the NATS pass (after a successful Cardinal pass) is returned.
func TestPruneCardinalImages_NATSListErrorPropagates(t *testing.T) {
	t.Parallel()

	fake := &fakeImageClient{
		listFn: func(opts client.ImageListOptions) (client.ImageListResult, error) {
			if _, ok := opts.Filters["label"]; ok {
				return client.ImageListResult{}, nil // Cardinal pass: no images, no error.
			}
			return client.ImageListResult{}, errors.New("nats list failed")
		},
	}

	err := pruneCardinalImages(context.Background(), fake, discardLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nats list failed")
}

// TestPruneCardinalImages_OnlyCardinalLabeledImagesTargeted verifies the
// label filter is honored: an unrelated (non-world-cli) image on the host is
// never listed for the Cardinal pass and thus never removed — even if it
// happens to share a repo-tag prefix with a Cardinal image.
func TestPruneCardinalImages_OnlyCardinalLabeledImagesTargeted(t *testing.T) {
	t.Parallel()

	fake := &fakeImageClient{
		images: []image.Summary{
			cardinalImage("sha256:card", "rampage-gameplay-shard:latest"),
			{ID: "sha256:foreign", RepoTags: []string{"rampage-gameplay-shard:latest"}, Labels: nil},
		},
	}

	require.NoError(t, pruneCardinalImages(context.Background(), fake, discardLogger()))
	require.Len(t, fake.removed, 1, "only the labeled image must be removed")
	assert.Equal(t, "sha256:card", fake.removed[0].imageRef)
}
