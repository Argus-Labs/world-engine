package docker

import (
	"context"
	"errors"
	"slices"
	"testing"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
)

// fakeVolumeClient tracks which volumes exist and records every VolumeInspect and VolumeRemove.
// A missing volume returns the daemon's not-found sentinel; inspectErr and removeFn inject other errors.
type fakeVolumeClient struct {
	existing   map[string]struct{}
	labels     map[string]map[string]string // labels of existing volumes; none when absent
	inspected  []string
	inspectErr error
	removed    []fakeVolumeRemoveCall
	removeFn   func(name string, opts client.VolumeRemoveOptions) error
}

func (f *fakeVolumeClient) VolumeInspect(
	_ context.Context,
	volumeID string,
	_ client.VolumeInspectOptions,
) (client.VolumeInspectResult, error) {
	f.inspected = append(f.inspected, volumeID)

	if f.inspectErr != nil {
		return client.VolumeInspectResult{}, f.inspectErr
	}

	if _, ok := f.existing[volumeID]; !ok {
		return client.VolumeInspectResult{}, cerrdefs.ErrNotFound
	}

	var res client.VolumeInspectResult

	res.Volume.Name = volumeID
	res.Volume.Labels = f.labels[volumeID]

	return res, nil
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

	require.NoError(t, removeVolumes(context.Background(), fake, "rampage", []string{
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

	require.NoError(t, removeVolumes(context.Background(), fake, "rampage", []string{
		"rampage-nats-data", "rampage-db-data", "rampage-meta-service",
	}))

	assert.Empty(t, fake.existing)
	assert.Equal(t,
		[]string{"rampage-nats-data", "rampage-db-data", "rampage-meta-service"},
		fake.inspected)
	assert.Equal(t, []string{"rampage-nats-data"}, fake.removedNames())
}

func TestRemoveVolumes_NotFoundIsNotAnError(t *testing.T) {
	t.Parallel()

	fake := &fakeVolumeClient{existing: map[string]struct{}{}}

	require.NoError(
		t,
		removeVolumes(context.Background(), fake, "rampage", []string{"rampage-nats-data", "rampage-db-data"}),
	)
	require.Len(t, fake.inspected, 2, "both names must still be attempted")
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

	err := removeVolumes(context.Background(), fake, "rampage", []string{"rampage-nats-data", "rampage-db-data"})
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

	err := removeVolumes(context.Background(), fake, "rampage", []string{
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

	require.NoError(t, removeVolumes(context.Background(), fake, "rampage", []string{"", "rampage-nats-data", ""}))
	assert.Equal(t, []string{"rampage-nats-data"}, fake.removedNames())
}

func TestRemoveVolumes_NoNamesIsNoOp(t *testing.T) {
	t.Parallel()

	fake := &fakeVolumeClient{existing: map[string]struct{}{}}
	require.NoError(t, removeVolumes(context.Background(), fake, "rampage", nil))
	assert.Empty(t, fake.removed)
}

// Project "demo" service "game-api" and project "demo-game" service "api" both name
// their volume "demo-game-api-service". Purging "demo" must keep the other project's.
func TestRemoveVolumes_KeepsAnotherProjectsVolume(t *testing.T) {
	t.Parallel()

	name := service.GameServiceContainerName("demo", "game-api")
	require.Equal(t, service.GameServiceContainerName("demo-game", "api"), name)

	cases := map[string]struct {
		labels      map[string]string
		inspectErr  error
		wantErr     string
		wantRemoved bool
	}{
		"own project's volume is removed": {
			labels:      map[string]string{service.ProjectLabel: "demo"},
			wantRemoved: true,
		},
		// Volumes from a CLI that predates the project label are still removed.
		"unlabelled volume is removed": {
			wantRemoved: true,
		},
		"other project's volume is kept": {
			labels:  map[string]string{service.ProjectLabel: "demo-game"},
			wantErr: `belongs to project "demo-game"`,
		},
		"volume that cannot be inspected is kept": {
			inspectErr: errors.New("daemon unavailable"),
			wantErr:    "inspect volume " + name,
		},
	}
	for desc, tc := range cases {
		t.Run(desc, func(t *testing.T) {
			t.Parallel()

			fake := &fakeVolumeClient{
				existing:   map[string]struct{}{name: {}, "demo-nats-data": {}},
				labels:     map[string]map[string]string{name: tc.labels},
				inspectErr: tc.inspectErr,
			}

			err := removeVolumes(context.Background(), fake, "demo", []string{name, "demo-nats-data"})
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}

			_, stillThere := fake.existing[name]
			assert.Equal(t, !tc.wantRemoved, stillThere)
			assert.Equal(t, tc.wantRemoved, slices.Contains(fake.removedNames(), name))
			assert.Equal(t, []string{name, "demo-nats-data"}, fake.inspected, "every name must still be attempted")
		})
	}
}
