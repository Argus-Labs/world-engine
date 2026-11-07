package docker

import (
	"context"
	"errors"
	"testing"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeVolumeClient tracks which volumes exist and records every VolumeRemove.
// A missing volume returns the daemon's not-found sentinel; removeFn injects other errors.
type fakeVolumeClient struct {
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
	var rerr error
	if f.removeFn != nil {
		rerr = f.removeFn(volumeID, opts)
	} else if _, ok := f.existing[volumeID]; !ok {
		rerr = cerrdefs.ErrNotFound
	}
	if rerr == nil {
		delete(f.existing, volumeID)
	}
	f.removed = append(f.removed, fakeVolumeRemoveCall{name: volumeID, force: opts.Force, err: rerr})
	return client.VolumeRemoveResult{}, rerr
}

func (f *fakeVolumeClient) removedNames() []string {
	out := make([]string, 0, len(f.removed))
	for _, c := range f.removed {
		out = append(out, c.name)
	}
	return out
}

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

	require.Len(t, fake.removed, 3)
	for _, call := range fake.removed {
		assert.True(t, call.force, "volume %q must be removed with Force", call.name)
		assert.NoError(t, call.err)
	}
	assert.Empty(t, fake.existing)
}

// An orphaned volume (container already gone) is reclaimed by name; never-created names are not-found skips.
func TestRemoveVolumes_OrphanedVolumeStillRemoved(t *testing.T) {
	t.Parallel()

	fake := &fakeVolumeClient{existing: map[string]struct{}{"rampage-nats-data": {}}}

	require.NoError(t, removeVolumes(context.Background(), fake, []string{
		"rampage-nats-data", "rampage-db-data", "rampage-meta-service",
	}))

	assert.Empty(t, fake.existing)
	assert.Equal(t,
		[]string{"rampage-nats-data", "rampage-db-data", "rampage-meta-service"},
		fake.removedNames())
}

func TestRemoveVolumes_NotFoundIsNotAnError(t *testing.T) {
	t.Parallel()

	fake := &fakeVolumeClient{existing: map[string]struct{}{}}

	require.NoError(t, removeVolumes(context.Background(), fake, []string{"rampage-nats-data", "rampage-db-data"}))
	require.Len(t, fake.removed, 2, "both names must still be attempted")
}

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

// Every volume is attempted and every failure is reported, so one run shows all leaked volumes.
func TestRemoveVolumes_AttemptsAllAndJoinsErrors(t *testing.T) {
	t.Parallel()

	stuckDB := errors.New("db volume is in use")
	stuckMeta := errors.New("meta volume is in use")
	fake := &fakeVolumeClient{
		existing: map[string]struct{}{
			"rampage-nats-data":    {},
			"rampage-db-data":      {},
			"rampage-meta-service": {},
		},
		removeFn: func(name string, _ client.VolumeRemoveOptions) error {
			switch name {
			case "rampage-db-data":
				return stuckDB
			case "rampage-meta-service":
				return stuckMeta
			}
			return nil
		},
	}

	err := removeVolumes(context.Background(), fake, []string{
		"rampage-nats-data", "rampage-db-data", "rampage-meta-service",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, stuckDB)
	assert.ErrorIs(t, err, stuckMeta)
	assert.Contains(t, err.Error(), "remove volume rampage-db-data")
	assert.Contains(t, err.Error(), "remove volume rampage-meta-service")
	require.Len(t, fake.removed, 3)
	assert.NotContains(t, fake.existing, "rampage-nats-data", "removable volumes are still reclaimed")
	assert.Contains(t, fake.existing, "rampage-db-data")
	assert.Contains(t, fake.existing, "rampage-meta-service")
}

func TestRemoveVolumes_EmptyNamesSkipped(t *testing.T) {
	t.Parallel()

	fake := &fakeVolumeClient{existing: map[string]struct{}{"rampage-nats-data": {}}}

	require.NoError(t, removeVolumes(context.Background(), fake, []string{"", "rampage-nats-data", ""}))
	assert.Equal(t, []string{"rampage-nats-data"}, fake.removedNames())
}

func TestRemoveVolumes_NoNamesIsNoOp(t *testing.T) {
	t.Parallel()

	fake := &fakeVolumeClient{existing: map[string]struct{}{}}
	require.NoError(t, removeVolumes(context.Background(), fake, nil))
	assert.Empty(t, fake.removed)
}
