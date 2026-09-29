package debugger

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"

	"github.com/argus-labs/world-engine/cli/pkg/cluster"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

const poolWorldToml = `organization = "test-org"
project = "test-project"

[[shards]]
id = "game"
pool_size = 2

[[shards]]
id = "meta"
`

func dirWithToml(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, worldtoml.FileName), []byte(content), 0o644)
	require.NoError(t, err)
	return dir
}

func TestResolveTargets(t *testing.T) {
	t.Parallel()

	t.Run("pairs selected instances with their URLs", func(t *testing.T) {
		t.Parallel()
		dir := dirWithToml(t, poolWorldToml)

		targets, err := resolveTargets(dir, []string{"game-2", "meta"})

		require.NoError(t, err)
		assert.Equal(t, []debugTarget{
			{
				instanceID: "game-2",
				url:        cluster.LocalShardAPIURL("test-org", "test-project", "game-2"),
			},
			{
				instanceID: "meta",
				url:        cluster.LocalShardAPIURL("test-org", "test-project", "meta"),
			},
		}, targets)
	})

	t.Run("missing world.toml", func(t *testing.T) {
		t.Parallel()

		_, err := resolveTargets(t.TempDir(), nil)

		require.Error(t, err)
		assert.ErrorContains(t, err, worldtoml.FileName)
	})
}

func TestRunDebugAction(t *testing.T) {
	dir := dirWithToml(t, poolWorldToml)
	t.Chdir(dir)

	t.Run("runs on selected instances", func(t *testing.T) {
		var mu sync.Mutex
		var called []string
		action := func(
			_ context.Context, _ cardinalv1connect.DebugServiceClient, instanceID string,
		) (string, error) {
			mu.Lock()
			called = append(called, instanceID)
			mu.Unlock()
			return "OK", nil
		}

		err := runDebugAction(context.Background(), []string{"game-2", "meta"}, "test", action)

		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"game-2", "meta"}, called)
	})

	t.Run("unknown instance prevents every action", func(t *testing.T) {
		var calls atomic.Int32
		action := func(
			_ context.Context, _ cardinalv1connect.DebugServiceClient, _ string,
		) (string, error) {
			calls.Add(1)
			return "", nil
		}

		err := runDebugAction(context.Background(), []string{"game", "nope"}, "pause", action)

		require.Error(t, err)
		assert.ErrorContains(t, err, `"nope"`)
		assert.Zero(t, calls.Load())
	})

	t.Run("partial failure attempts every instance", func(t *testing.T) {
		var mu sync.Mutex
		var called []string
		action := func(
			_ context.Context, _ cardinalv1connect.DebugServiceClient, instanceID string,
		) (string, error) {
			mu.Lock()
			called = append(called, instanceID)
			mu.Unlock()
			if instanceID == "meta" {
				return "", eris.New("timeout")
			}
			return "OK", nil
		}

		err := runDebugAction(context.Background(), nil, "step", action)

		require.Error(t, err)
		assert.ErrorContains(t, err, "1 of 3")
		assert.ErrorContains(t, err, "step")
		assert.ElementsMatch(t, []string{"game", "game-2", "meta"}, called)
	})
}
