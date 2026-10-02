//go:build integration

package root

import (
	"context"
	"io"
	"testing"

	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRemoveRegistryPushTags_ServiceAndShard is the integration test for the
// purge pass-2 fix. It exercises removeRegistryPushTags against a real Docker
// daemon to prove that:
//
//   - A path-kind service image carrying the shared :latest source tag AND
//     multiple accumulated :local-<ts> registry push tags (the cached-build
//     steady state from the bug report) is fully reclaimed by pass 2's
//     ImageRemove(img.ID, Force: true). Pass 1 (PruneCardinalImages, no Force)
//     cannot remove such a multi-tagged image; pass 2 can, and now it actually
//     iterates service IDs (the bug was that it never did).
//   - A shard image is removed the same way (no regression in the shard path).
//   - Union of shard + service IDs in one call removes both (the wiring the fix
//     adds in purgeK8sImages).
//
// It mirrors the build-tag + runtime-skip convention of
// cli/pkg/docker/client_integration_test.go: it self-skips if the Docker daemon
// is unreachable. It builds NO real images — it retags a tiny base image
// (hello-world) into the k3d-registry reference shape DeployServices creates, so
// the test is fast and self-contained. All images/tags it creates are asserted
// removed by the function under test, so nothing leaks if it passes.
func TestRemoveRegistryPushTags_ServiceAndShard(t *testing.T) {
	ctx := context.Background()
	dockerCli, err := client.New(client.FromEnv)
	if err != nil {
		t.Skipf("skipping purge integration test; Docker not available: %v", err)
	}
	defer func() { _ = dockerCli.Close() }()

	// Use the same registry name cluster.Config defaults to (see
	// cli/pkg/cluster/config.go: "world-engine-registry"), so the reference shape
	// is byte-identical to imageRef()/DeployServices. project is arbitrary.
	const (
		registryName = "world-engine-registry"
		project      = "purge-integration"
	)

	// mkImage pulls a tiny base image (baseRef) fresh, then retags its ID into
	// a fake "built source image" that carries:
	//   - the shared :latest source tag (<project>-<id>-service:latest), and
	//   - one or more :local-<ts> registry push tags on the SAME image ID.
	// This mirrors the post-DeployServices state of a path-kind service image.
	// The base ref is parametrized so callers can use DISTINCT base images when
	// they need the resulting image IDs to differ (e.g. the union test, where two
	// IDs retagging the same base would collapse to one image ID and be removed
	// together rather than per-ID).
	//
	// A fresh pull per call is intentional: pass 2's ImageRemove(img.ID, Force)
	// reclaims the WHOLE image (all repo tags, including the base we retagged
	// from), so reusing one base ID across subtests would leave later subtests
	// with "no such image". Re-pulling is cheap — the layers stay cached locally.
	mkImage := func(t *testing.T, id, baseRef string, localTags ...string) string {
		t.Helper()
		baseID := pullAndID(t, dockerCli, ctx, baseRef)
		// Source image name, matching serviceContainerName/gameServiceContainerName.
		source := project + "-" + id + "-service:latest"
		if _, err := dockerCli.ImageTag(ctx, client.ImageTagOptions{Source: baseID, Target: source}); err != nil {
			t.Fatalf("tag source %s for %s: %v", source, id, err)
		}
		for _, lt := range localTags {
			dest := "k3d-" + registryName + ".localhost:5000/" + project + "/" + id + ":" + lt
			if _, err := dockerCli.ImageTag(ctx, client.ImageTagOptions{Source: baseID, Target: dest}); err != nil {
				t.Fatalf("tag registry %s for %s: %v", dest, id, err)
			}
		}
		return "k3d-" + registryName + ".localhost:5000/" + project + "/" + id
	}

	// listByRef returns the images matching the k3d-registry reference filter
	// pass 2 uses (reference=<ref>:*).
	listByRef := func(ref string) []string {
		t.Helper()
		list, lerr := dockerCli.ImageList(ctx, client.ImageListOptions{
			Filters: client.Filters{}.Add("reference", ref+":*"),
		})
		require.NoError(t, lerr, "ImageList with reference filter %s", ref)
		ids := make([]string, 0, len(list.Items))
		for _, im := range list.Items {
			ids = append(ids, im.ID)
		}
		return ids
	}

	t.Run("service_image_with_multiple_local_tags_is_fully_reclaimed", func(t *testing.T) {
		const svcID = "meta"
		ref := mkImage(t, svcID, "hello-world:latest", "local-1", "local-2") // 2 accumulated per-start tags + :latest = 3 tags on one ID
		ids := listByRef(ref)
		require.Len(t, ids, 1, "expected exactly one image tagged under %s", ref)
		imgID := ids[0]

		// BEFORE: pass 1 (no Force) CANNOT remove this image — it has 2+ repo tags.
		_, removeErr := dockerCli.ImageRemove(ctx, imgID, client.ImageRemoveOptions{PruneChildren: true})
		require.Error(t, removeErr, "pass 1 (no Force) must fail for a multi-tagged image — the bug's root cause")

		// Pass 2 (the fix): iterate the service ID, Force-remove by image ID.
		removed, err := removeRegistryPushTags(ctx, registryName, project, []string{svcID})
		require.NoError(t, err)
		assert.Equal(t, 1, removed, "pass 2 should remove the one service image")

		// AFTER: registry ref gone, and the image ID itself is gone (Force removed
		// the whole image, including the shared :latest source tag).
		assert.Empty(t, listByRef(ref), "registry push tag for service %s must be gone", svcID)
		_, inspectErr := dockerCli.ImageInspect(ctx, imgID)
		assert.Error(t, inspectErr, "the whole service image (incl. :latest) must be reclaimed")
	})

	t.Run("shard_image_still_removed", func(t *testing.T) {
		const shardID = "gameplay"
		ref := mkImage(t, shardID, "hello-world:latest", "local-1")
		removed, err := removeRegistryPushTags(ctx, registryName, project, []string{shardID})
		require.NoError(t, err)
		assert.Equal(t, 1, removed, "shard image must still be removed (no regression)")
		assert.Empty(t, listByRef(ref))
	})

	t.Run("union_of_shard_and_service_ids_removes_both", func(t *testing.T) {
		const shardID, svcID = "chat", "ranks"
		// DISTINCT base images so the two image IDs differ — required to prove
		// pass 2 iterates BOTH ids in the union. (Same base would collapse to one
		// image ID removed once, masking whether the service ID was iterated.)
		_ = mkImage(t, shardID, "hello-world:latest", "local-1")
		_ = mkImage(t, svcID, "busybox:latest", "local-1", "local-2")

		// The exact union purgeK8sImages now builds.
		ids := []string{shardID, svcID}
		removed, err := removeRegistryPushTags(ctx, registryName, project, ids)
		require.NoError(t, err)
		assert.Equal(t, 2, removed, "both shard and service images must be removed")

		for _, id := range ids {
			ref := "k3d-" + registryName + ".localhost:5000/" + project + "/" + id
			assert.Empty(t, listByRef(ref), "%s image must be gone", id)
		}
	})

	// Cleanup any stragglers from a failed run so the test stays idempotent.
	t.Cleanup(func() {
		for _, id := range []string{"meta", "gameplay", "chat", "ranks"} {
			_, _ = removeRegistryPushTags(ctx, registryName, project, []string{id})
			_, _ = dockerCli.ImageRemove(ctx, project+"-"+id+"-service:latest",
				client.ImageRemoveOptions{Force: true, PruneChildren: true})
		}
	})
}

// pullAndID ensures an image is present and returns its ID. ImagePull returns a
// body stream that MUST be drained (and closed) for the pull to complete and the
// image to appear in ImageList — mirroring pkg/docker/client_image.go's drain.
func pullAndID(t *testing.T, dockerCli *client.Client, ctx context.Context, ref string) string {
	t.Helper()
	resp, err := dockerCli.ImagePull(ctx, ref, client.ImagePullOptions{})
	if err != nil {
		t.Logf("ImagePull %s: %v (continuing — image may already be present)", ref, err)
	} else {
		_, _ = io.Copy(io.Discard, resp)
		_ = resp.Close()
	}
	list, err := dockerCli.ImageList(ctx, client.ImageListOptions{
		Filters: client.Filters{}.Add("reference", ref),
	})
	require.NoError(t, err, "ImageList for %s", ref)
	require.NotEmpty(t, list.Items, "no image found for %s after pull", ref)
	return list.Items[0].ID
}
