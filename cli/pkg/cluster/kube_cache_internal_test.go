// These tests pin the kube-client cache lifecycle that a long-lived
// *cluster.Client (the MCP server's process-wide singleton) depends on:
// ensureCluster must invalidate the memoized kubeClient when it creates a
// fresh cluster (k3dCreate picks a new randomized API port), and must NOT
// invalidate it when the cluster already exists (stop/start keeps the same
// port). The k3d package-level function vars are swapped with fakes so no
// Docker/k3d runtime is required.
//
// This file uses the internal `package cluster` (not `cluster_test`) because
// it exercises unexported fields/methods: c.cachedKube, c.ensureCluster,
// c.kube, and the k3d function vars. The other internal test files in this
// package (k3d_essential_test.go, kubeapply_test.go, ...) carry the same
// `package cluster` choice for the same reason.
//
// Tests that swap the k3d globals are intentionally NOT marked t.Parallel:
// the globals are shared process state, and parallel swaps would race. They
// run sequentially before the package's parallel tests, and t.Cleanup
// restores the originals so no global is left swapped for later tests.
package cluster

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeKubeconfigYAML returns a minimal valid kubeconfig whose server URL
// points at https://127.0.0.1:<port>. newKubeClient parses it (via
// clientcmd.RESTConfigFromKubeConfig) and builds the dynamic/discovery clients
// without connecting, so a test can build distinct kubeClients for distinct
// ports and assert kube() picks the right one after an invalidate.
func fakeKubeconfigYAML(port int) []byte {
	return []byte(fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:%s
  name: test
contexts:
- context:
    cluster: test
    user: test
  name: test
current-context: test
users:
- name: test
  user: {}
`, strconv.Itoa(port)))
}

// k3dFakes holds optional overrides for the package-level k3d function vars.
// nil fields leave the real implementation in place.
type k3dFakes struct {
	exists         func(context.Context, string) (bool, error)
	create         func(context.Context, string, string, string) error
	startIfStopped func(context.Context, string) error
	kubeconfig     func(context.Context, string) ([]byte, error)
	del            func(context.Context, string) error
}

// swapK3d replaces the package-level k3d function vars with the supplied
// fakes and registers cleanup to restore the originals. Non-nil fields are
// applied; nil fields are skipped. MUST be paired with a non-parallel test.
func swapK3d(tb testing.TB, f k3dFakes) {
	tb.Helper()
	orig := k3dFakes{
		exists:         k3dExists,
		create:         k3dCreate,
		startIfStopped: k3dStartIfStopped,
		kubeconfig:     k3dKubeconfig,
		del:            k3dDelete,
	}
	tb.Cleanup(func() { restoreK3d(orig) })
	applyK3dFakes(f)
}

func restoreK3d(orig k3dFakes) {
	k3dExists = orig.exists
	k3dCreate = orig.create
	k3dStartIfStopped = orig.startIfStopped
	k3dKubeconfig = orig.kubeconfig
	k3dDelete = orig.del
}

func applyK3dFakes(f k3dFakes) {
	if f.exists != nil {
		k3dExists = f.exists
	}
	if f.create != nil {
		k3dCreate = f.create
	}
	if f.startIfStopped != nil {
		k3dStartIfStopped = f.startIfStopped
	}
	if f.kubeconfig != nil {
		k3dKubeconfig = f.kubeconfig
	}
	if f.del != nil {
		k3dDelete = f.del
	}
}

// newClientWithCache builds a *Client whose cachedKube is already populated
// with a kubeClient whose restCfg.Host points at https://127.0.0.1:<port>,
// simulating a prior successful StartPlatform against a cluster at <port>.
// It goes through the real kube() path (with a mocked k3dKubeconfig) so the
// cached client is a genuine *kubeClient built by newKubeClient.
func newClientWithCache(tb testing.TB, port int) *Client {
	tb.Helper()
	swapK3d(tb, k3dFakes{
		kubeconfig: func(_ context.Context, _ string) ([]byte, error) {
			return fakeKubeconfigYAML(port), nil
		},
	})
	c := NewClient(Config{})
	k, err := c.kube(context.Background())
	require.NoError(tb, err)
	require.NotNil(tb, k)
	require.Equal(tb, "https://127.0.0.1:"+strconv.Itoa(port), k.restCfg.Host)
	return c
}

// cachedKubePtr returns the current cachedKube pointer (nil-safe) under the
// Client's mutex, for assertions that don't go through kube().
func cachedKubePtr(c *Client) *kubeClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cachedKube
}

// cachedKubeHost returns the restCfg.Host of the cached kubeClient, or "" if
// the cache is empty.
func cachedKubeHost(c *Client) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cachedKube == nil || c.cachedKube.restCfg == nil {
		return ""
	}
	return c.cachedKube.restCfg.Host
}

// TestEnsureCluster_CreateBranch_InvalidatesCachedKube is the core regression
// test for the bug: when ensureCluster takes the "cluster doesn't exist →
// create" branch, it MUST drop the memoized kubeClient so the next kube()
// rebuilds from the new cluster's kubeconfig (whose server URL carries a
// fresh randomized API port). Before the fix, the stale client survived the
// recreate and StartPlatform's CRD/platform apply hit a dead endpoint.
func TestEnsureCluster_CreateBranch_InvalidatesCachedKube(t *testing.T) {
	const oldPort = 34567
	c := newClientWithCache(t, oldPort)
	require.NotNil(t, cachedKubePtr(c), "precondition: cache must be populated")

	var createCalled atomic.Bool
	swapK3d(t, k3dFakes{
		exists: func(_ context.Context, _ string) (bool, error) { return false, nil },
		create: func(_ context.Context, _, _, _ string) error {
			createCalled.Store(true)
			return nil
		},
	})

	require.NoError(t, c.ensureCluster(context.Background()))

	require.True(t, createCalled.Load(), "k3dCreate must be invoked on the create branch")
	require.Nil(t, cachedKubePtr(c),
		"cachedKube must be nil after a fresh create so the next kube() rebuilds")
}

// TestEnsureCluster_ExistingBranch_PreservesCachedKube guards that the
// "cluster exists → start-if-stopped" branch does NOT invalidate the cache.
// k3dStop/k3dStart keep the same cluster and therefore the same API port, so
// the memoized client stays valid. Invalidating here would force a needless
// kubeconfig refetch on every warm start.
func TestEnsureCluster_ExistingBranch_PreservesCachedKube(t *testing.T) {
	const port = 34567
	c := newClientWithCache(t, port)
	before := cachedKubePtr(c)
	require.NotNil(t, before, "precondition")

	var startCalled atomic.Bool
	swapK3d(t, k3dFakes{
		exists: func(_ context.Context, _ string) (bool, error) { return true, nil },
		startIfStopped: func(_ context.Context, _ string) error {
			startCalled.Store(true)
			return nil
		},
	})

	require.NoError(t, c.ensureCluster(context.Background()))
	require.True(t, startCalled.Load(), "k3dStartIfStopped must be invoked on the existing branch")
	require.Same(t, before, cachedKubePtr(c),
		"cache must be preserved when the cluster already exists (same API port after stop/start)")
	require.Equal(t, "https://127.0.0.1:"+strconv.Itoa(port), cachedKubeHost(c))
}

// TestEnsureCluster_CreateThenKube_RebuildsFromNewEndpoint is the end-to-end
// test for the reported scenario: a long-lived *Client with a cached client
// at port A, an out-of-band cluster delete, then a StartPlatform-equivalent
// sequence (ensureCluster creates at port B, kube() returns a client for B).
// Before the fix, kube() returned the stale client at A and the apply failed
// with connection-refused; after the fix it returns the rebuilt client at B.
func TestEnsureCluster_CreateThenKube_RebuildsFromNewEndpoint(t *testing.T) {
	const oldPort, newPort = 34567, 38888

	// Seed the cache with a client for the OLD cluster's API port.
	c := newClientWithCache(t, oldPort)
	require.Equal(t, "https://127.0.0.1:"+strconv.Itoa(oldPort), cachedKubeHost(c))

	// Simulate the out-of-band delete (cluster gone) + fresh recreate (new port).
	// k3dCreate is a no-op; the new kubeconfig is served by the swapped
	// k3dKubeconfig below when kube() rebuilds.
	swapK3d(t, k3dFakes{
		exists: func(_ context.Context, _ string) (bool, error) { return false, nil },
		create: func(_ context.Context, _, _, _ string) error { return nil },
		kubeconfig: func(_ context.Context, _ string) ([]byte, error) {
			return fakeKubeconfigYAML(newPort), nil
		},
	})

	require.NoError(t, c.ensureCluster(context.Background()))
	require.Nil(t, cachedKubePtr(c), "cache must be cleared before/as k3dCreate runs")

	k, err := c.kube(context.Background())
	require.NoError(t, err)
	require.NotNil(t, k)
	require.Equal(t, "https://127.0.0.1:"+strconv.Itoa(newPort), k.restCfg.Host,
		"kube() must return a client for the newly-created cluster, not the stale one")
	require.Equal(t, "https://127.0.0.1:"+strconv.Itoa(newPort), cachedKubeHost(c),
		"the rebuilt client must be memoized for reuse")
	require.NotEqual(t, "https://127.0.0.1:"+strconv.Itoa(oldPort), k.restCfg.Host,
		"the rebuilt client must NOT point at the dead old port")
}

// TestPurge_NoCluster_InvalidatesCachedKube pins the defense-in-depth fix: a
// no-op Purge (cluster already gone, e.g. after an out-of-band docker rm)
// reconciles a stale cache with reality so the next caller doesn't inherit
// a dead client. Before this addition, Purge's !exists early return left a
// stale cache intact.
func TestPurge_NoCluster_InvalidatesCachedKube(t *testing.T) {
	c := newClientWithCache(t, 34567)
	require.NotNil(t, cachedKubePtr(c), "precondition")

	var deleteCalled atomic.Bool
	swapK3d(t, k3dFakes{
		exists: func(_ context.Context, _ string) (bool, error) { return false, nil },
		del: func(_ context.Context, _ string) error {
			deleteCalled.Store(true)
			return nil
		},
	})

	require.NoError(t, c.Purge(context.Background(), PurgeOpts{}))
	require.False(t, deleteCalled.Load(), "k3dDelete must not run when the cluster is absent")
	require.Nil(t, cachedKubePtr(c),
		"a no-op Purge must still reconcile the stale cache with reality")
}
