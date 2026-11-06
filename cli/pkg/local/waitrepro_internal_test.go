package local

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// startDeadListener holds a port with an accept-and-close loop, mimicking
// Docker's port proxy in front of a container that is not serving: a TCP dial
// succeeds but an HTTP request must not. This is what a crashed sibling looks
// like to WaitForShardsReady's probes.
func startDeadListener(t *testing.T, port string) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		t.Skipf("port %s unavailable: %v", port, err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	return ln
}

// startServingListener binds a real HTTP server on a port, so shardReady counts
// it as ready (it replies to an HTTP request, even with 404).
func startServingListener(t *testing.T, port string) *http.Server {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		t.Skipf("port %s unavailable: %v", port, err)
	}
	srv := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.NotFoundHandler()}
	go func() { _ = srv.Serve(ln) }()
	return srv
}

// poolShardConfig builds a 2-instance "game" pool (ports 8081–8082) for the
// reproduction tests.
func poolShardConfig() *service.Config {
	return &service.Config{Project: "demo", WorldToml: worldtoml.Config{
		Project: "demo",
		Shards: []worldtoml.Shard{
			{ID: "game", InstanceID: "game"},   // ShardHostPort(0) = 8081
			{ID: "game", InstanceID: "game-2"}, // ShardHostPort(1) = 8082
		},
	}}
}

// TestWaitForShardsReadySingleInstanceBlindSpot reproduces the bug mechanism:
// when only the user-named instance is probed, WaitForShardsReady returns
// ready==expected (success) even though a sibling recreated by the deploy is
// dead. The deploy recreates the whole pool, so the wait must cover every
// sibling — but WaitForShardsReady itself is correct; the bug was in the caller
// (rollShards) passing too narrow a list. This test pins the mechanism the fix
// relies on: a single-instance scope cannot see a dead sibling.
func TestWaitForShardsReadySingleInstanceBlindSpot(t *testing.T) {
	r := New(nil, poolShardConfig())

	// Serve real HTTP only on game-2's port (8082); game (8081) is dead.
	srv := startServingListener(t, "8082")
	defer srv.Close()
	deadLn := startDeadListener(t, "8081")
	defer deadLn.Close()

	// CLI scope (before fix): only the user-named instance.
	// ready=1, expected=1 → false success while game is dead.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ready, expected := r.WaitForShardsReady(ctx, []string{"game-2"}, nil)
	assert.Equal(t, 1, expected, "single-instance scope: expected should be 1")
	assert.Equal(t, 1, ready, "single-instance scope: ready should be 1 (game-2 is serving)")
	// The blind spot: ready == expected, so the command would exit 0
	// even though game is dead.
}

// TestWaitForShardsReadyScopedWaitCountsDeadSiblings verifies that expanding the
// wait to every pool instance (the fix) makes WaitForShardsReady detect the dead
// sibling: ready < expected, triggering the shardsNotReadyError path.
func TestWaitForShardsReadyScopedWaitCountsDeadSiblings(t *testing.T) {
	r := New(nil, poolShardConfig())

	// Serve real HTTP only on game-2's port (8082); game (8081) is dead.
	srv := startServingListener(t, "8082")
	defer srv.Close()
	deadLn := startDeadListener(t, "8081")
	defer deadLn.Close()

	// Fixed scope: every instance of the deployed pool.
	// ready=1, expected=2 → ready < expected → shardsNotReadyError path.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var lastReady, lastExpected int
	ready, expected := r.WaitForShardsReady(ctx, []string{"game", "game-2"},
		func(ready, expected int) {
			lastReady, lastExpected = ready, expected
		})
	assert.Equal(t, 2, expected, "pool-wide scope: expected should be 2 (both instances)")
	assert.Equal(t, 1, ready, "pool-wide scope: ready should be 1 (only game-2 is serving)")
	assert.Less(t, ready, expected, "the dead sibling must make ready < expected")
	// onProgress should have reported the shortfall.
	assert.Equal(t, 1, lastReady)
	assert.Equal(t, 2, lastExpected)
}

// TestWaitForShardsReadyAllReadyReturnsImmediately verifies that when every
// probed instance is serving, WaitForShardsReady returns ready==expected
// without waiting for the deadline — the normal happy path after a successful
// reload.
func TestWaitForShardsReadyAllReadyReturnsImmediately(t *testing.T) {
	r := New(nil, poolShardConfig())

	// Both instances serving.
	srv1 := startServingListener(t, "8081")
	defer srv1.Close()
	srv2 := startServingListener(t, "8082")
	defer srv2.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	ready, expected := r.WaitForShardsReady(ctx, []string{"game", "game-2"}, nil)
	elapsed := time.Since(start)
	assert.Equal(t, 2, expected)
	assert.Equal(t, 2, ready)
	assert.Less(t, elapsed, 2*time.Second, "all ready should return well before the deadline")
}
