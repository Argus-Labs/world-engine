package docker

import (
	"context"
	"errors"
	"sync"
	"testing"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeVolumeClient is a stand-in dockerVolumeClient for unit-testing the purge
// volume sweep without a running daemon. It tracks which volume names still
// "exist" and records every VolumeRemove call. By default a missing volume
// returns the same not-found sentinel the real daemon reports (so the
// not-found skip path is exercised); removeFn lets a test inject other failures
// such as an in-use volume or a daemon error.
type fakeVolumeClient struct {
	mu       sync.Mutex
	existing map[string]struct{}
	removed  []fakeVolumeRemoveCall
	removeFn func(name string, opts client.VolumeRemoveOptions) error
}

type fakeVolumeRemoveCall struct {
	name  string
	force bool
	err   error
}

func (f *fakeVolumeClient) VolumeRemove(
	_ context.Context,
	volumeID string,
	opts client.VolumeRemoveOptions,
) (client.VolumeRemoveResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var rerr error
	if f.removeFn != nil {
		rerr = f.removeFn(volumeID, opts)
	} else if _, ok := f.existing[volumeID]; !ok {
		// A missing volume reports the same not-found sentinel the real daemon
		// does, so the not-found skip path in removeVolumes is exercised.
		rerr = cerrdefs.ErrNotFound
	}
	if rerr == nil {
		delete(f.existing, volumeID)
	}
	f.removed = append(f.removed, fakeVolumeRemoveCall{name: volumeID, force: opts.Force, err: rerr})
	return client.VolumeRemoveResult{}, rerr
}

// removedNames is a test helper that flattens the recorded removal calls into
// the ordered list of volume names the sweep asked the daemon to delete.
func (f *fakeVolumeClient) removedNames() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.removed))
	for _, c := range f.removed {
		out = append(out, c.name)
	}
	return out
}

// TestRemoveVolumes_RemovesEveryNamedVolumeWithForce is the primary regression
// guard: purge sweeps the project's volumes by name so a volume whose container
// was already removed — invisible to the mount walk in RemoveContainers — is
// still reclaimed. Every removal must use Force so a volume referenced by a
// stopped container (an unremoved leftover) is deleted too.
func TestRemoveVolumes_RemovesEveryNamedVolumeWithForce(t *testing.T) {
	t.Parallel()

	fake := &fakeVolumeClient{existing: map[string]struct{}{
		"rampage-nats-data":    {},
		"rampage-db-data":      {},
		"rampage-meta-service": {},
	}}

	require.NoError(t, removeVolumes(context.Background(), fake, []string{
		"rampage-nats-data", "rampage-db-data", "rampage-meta-service",
	}))

	require.Len(t, fake.removed, 3, "every named volume must be removed")
	for _, call := range fake.removed {
		assert.True(t, call.force, "volume %q must be removed with Force", call.name)
		assert.NoError(t, call.err, "existing volume %q must be removed", call.name)
	}
	assert.Empty(t, fake.existing, "no named volume may survive purge")
}

// TestRemoveVolumes_OrphanedVolumeStillRemoved pins the exact bug: an orphaned
// volume — one whose container was already removed, so the mount walk cannot
// reach it — is removed by the name sweep. removeVolumes operates purely on
// names, so it reclaims the orphaned nats volume even though no container path
// is involved; the other two names were never created and are not-found skips.
func TestRemoveVolumes_OrphanedVolumeStillRemoved(t *testing.T) {
	t.Parallel()

	fake := &fakeVolumeClient{existing: map[string]struct{}{
		"rampage-nats-data": {},
	}}

	require.NoError(t, removeVolumes(context.Background(), fake, []string{
		"rampage-nats-data", "rampage-db-data", "rampage-meta-service",
	}))

	assert.NotContains(t, fake.existing, "rampage-nats-data", "orphaned volume must be reclaimed")
	assert.Empty(t, fake.existing, "the never-created volumes were not-found, not leaked")
	// Every name is still forwarded, in order, so the orphaned one is not skipped.
	assert.Equal(
		t,
		[]string{"rampage-nats-data", "rampage-db-data", "rampage-meta-service"},
		fake.removedNames(),
	)
}

// TestRemoveVolumes_NotFoundIsNotAnError: purge removes volumes that were never
// created (a service declared in world.toml whose start failed) or already
// wiped (the mount walk in RemoveContainers deleted them). A not-found must be
// skipped silently so purge still reports success when the only "missing"
// volumes are expected ones.
func TestRemoveVolumes_NotFoundIsNotAnError(t *testing.T) {
	t.Parallel()

	fake := &fakeVolumeClient{existing: map[string]struct{}{}} // nothing exists

	require.NoError(t, removeVolumes(context.Background(), fake, []string{
		"rampage-nats-data", "rampage-db-data",
	}))
	require.Len(t, fake.removed, 2, "both names must still be attempted")
	for _, call := range fake.removed {
		assert.True(t, call.force, "removal %q must still request Force", call.name)
	}
}

// TestRemoveVolumes_RealErrorIsReturned: a non-not-found failure (daemon down,
// volume in use by a running container) must be returned rather than
// swallowed, so the CLI does not print "(all state wiped)" for a purge that
// leaked a volume.
func TestRemoveVolumes_RealErrorIsReturned(t *testing.T) {
	t.Parallel()

	boom := errors.New("volume is in use")
	fake := &fakeVolumeClient{
		existing: map[string]struct{}{"rampage-nats-data": {}},
		removeFn: func(name string, _ client.VolumeRemoveOptions) error {
			if name == "rampage-nats-data" {
				return boom
			}
			return nil
		},
	}

	err := removeVolumes(context.Background(), fake, []string{"rampage-nats-data", "rampage-db-data"})
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "remove volume rampage-nats-data")
}

// TestRemoveVolumes_AttemptsAllAndReturnsFirstError: one stuck volume must not
// strand the others — every volume is still attempted so the most state is
// reclaimed — and the first error is returned so the failure is reported, not
// masked as success.
func TestRemoveVolumes_AttemptsAllAndReturnsFirstError(t *testing.T) {
	t.Parallel()

	stuck := errors.New("volume is in use")
	fake := &fakeVolumeClient{
		existing: map[string]struct{}{
			"rampage-nats-data":    {},
			"rampage-db-data":      {},
			"rampage-meta-service": {},
		},
		removeFn: func(name string, _ client.VolumeRemoveOptions) error {
			if name == "rampage-db-data" {
				return stuck
			}
			return nil
		},
	}

	err := removeVolumes(context.Background(), fake, []string{
		"rampage-nats-data", "rampage-db-data", "rampage-meta-service",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, stuck)
	// All three were attempted, so nats and meta were still reclaimed.
	require.Len(t, fake.removed, 3)
	assert.NotContains(t, fake.existing, "rampage-nats-data", "removable volumes must still be reclaimed")
	assert.NotContains(t, fake.existing, "rampage-meta-service")
	assert.Contains(t, fake.existing, "rampage-db-data", "the stuck volume stays behind")
}

// TestRemoveVolumes_EmptyNamesSkipped: an empty name (should never happen, but
// defensive) is not sent to the daemon — removing "" is not a meaningful op and
// the daemon would reject it.
func TestRemoveVolumes_EmptyNamesSkipped(t *testing.T) {
	t.Parallel()

	fake := &fakeVolumeClient{existing: map[string]struct{}{"rampage-nats-data": {}}}

	require.NoError(t, removeVolumes(context.Background(), fake, []string{"", "rampage-nats-data", ""}))
	assert.Equal(t, []string{"rampage-nats-data"}, fake.removedNames(),
		"empty names must not be forwarded to the daemon")
}

// TestRemoveVolumes_NoNamesIsNoOp: an empty list is a no-op, not an error.
func TestRemoveVolumes_NoNamesIsNoOp(t *testing.T) {
	t.Parallel()

	fake := &fakeVolumeClient{existing: map[string]struct{}{}}
	require.NoError(t, removeVolumes(context.Background(), fake, nil))
	assert.Empty(t, fake.removed)
}
