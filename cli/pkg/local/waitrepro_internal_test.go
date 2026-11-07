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

// startDeadListener binds an accept-and-close loop on a free local port,
// mimicking Docker's port proxy in front of a container that is not serving: a
// TCP dial succeeds but an HTTP request must not. This is what a crashed sibling
// looks like to WaitForShardsReady's probes. Returns the listener (close to stop)
// and the bound port. Shared with the probe tests so the readiness test files
// cannot drift on the listener contract.
func startDeadListener(t *testing.T) (net.Listener, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
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
	return ln, ln.Addr().(*net.TCPAddr).Port
}

// startServingListener binds a real HTTP server on a free local port, so
// shardReady counts it as ready (it replies to an HTTP request, even with 404).
// Returns the server (close to stop) and the bound port.
func startServingListener(t *testing.T) (*http.Server, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.NotFoundHandler()}
	go func() { _ = srv.Serve(ln) }()
	return srv, ln.Addr().(*net.TCPAddr).Port
}

// poolShardConfig builds a 2-instance "game" pool for the reproduction tests.
// Ports are not in the config — the waiter derives them via the Runtime's
// shardHostPort seam, which the tests override to free ports so a running local
// world cannot collide.
func poolShardConfig() *service.Config {
	return &service.Config{Project: "demo", WorldToml: worldtoml.Config{
		Project: "demo",
		Shards: []worldtoml.Shard{
			{ID: "game", InstanceID: "game"},
			{ID: "game", InstanceID: "game-2"},
		},
	}}
}

// withFreePorts binds a dead listener and a serving listener on free ports, then
// points the Runtime's probeHostPort seam at them (one port per shard instance, in
// world.toml order) for the duration of fn, so the waiter probes real listeners
// without touching the production 8081+i ports a developer's running world may
// already hold.
func withFreePorts(t *testing.T, r *Runtime, fn func()) {
	t.Helper()
	deadLn, p0 := startDeadListener(t)
	defer deadLn.Close()
	srv, p1 := startServingListener(t)
	defer srv.Close()

	r.probeHostPort = func(i int) int { return []int{p0, p1}[i] }
	fn()
}

// TestWaitForShardsReadyWaitScope reproduces the bug mechanism and its fix in one
// table: game (instance 0) is dead and game-2 (instance 1) is serving, so the
// listener setup is shared and only the wait scope (the instance IDs handed to
// WaitForShardsReady) and the expected counts change.
//
// The SingleInstanceBlindSpot case is the bug: probing only the user-named
// instance returns ready==expected (false success) while a sibling recreated by
// the deploy is dead. The fix lives in the caller (rollShards), which must pass
// the whole pool; this pins the mechanism the fix relies on.
//
// The ScopedWaitCountsDeadSiblings case is the fixed scope: probing every
// instance of the deployed pool returns ready<expected, the path that surfaces
// shardsNotReadyError.
func TestWaitForShardsReadyWaitScope(t *testing.T) {
	r := New(nil, poolShardConfig())
	withFreePorts(t, r, func() {
		tests := []struct {
			name          string
			instanceIDs   []string
			wantReady     int
			wantExpected  int
			checkProgress bool
		}{
			{
				name:         "SingleInstanceBlindSpot",
				instanceIDs:  []string{"game-2"},
				wantReady:    1,
				wantExpected: 1, // ready == expected: false success while game is dead
			},
			{
				name:          "ScopedWaitCountsDeadSiblings",
				instanceIDs:   []string{"game", "game-2"},
				wantReady:     1,
				wantExpected:  2, // ready < expected → shardsNotReadyError path
				checkProgress: true,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				var lastReady, lastExpected int
				ready, expected := r.WaitForShardsReady(ctx, tt.instanceIDs,
					func(ready, expected int) {
						lastReady, lastExpected = ready, expected
					})
				assert.Equal(t, tt.wantExpected, expected, "expected")
				assert.Equal(t, tt.wantReady, ready, "ready")
				if tt.checkProgress {
					assert.Equal(t, tt.wantReady, lastReady, "onProgress ready")
					assert.Equal(t, tt.wantExpected, lastExpected, "onProgress expected")
				}
			})
		}
	})
}

// TestWaitForShardsReadyAllReadyReturnsImmediately verifies that when every
// probed instance is serving, WaitForShardsReady returns ready==expected
// without waiting for the deadline — the normal happy path after a successful
// reload.
func TestWaitForShardsReadyAllReadyReturnsImmediately(t *testing.T) {
	srv1, p0 := startServingListener(t)
	defer srv1.Close()
	srv2, p1 := startServingListener(t)
	defer srv2.Close()

	r := New(nil, poolShardConfig())
	r.probeHostPort = func(i int) int { return []int{p0, p1}[i] }
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ready, expected := r.WaitForShardsReady(ctx, []string{"game", "game-2"}, nil)
	assert.Equal(t, 2, expected)
	assert.Equal(t, 2, ready)
	assert.NoError(t, ctx.Err(), "healthy pool should complete before the deadline")
}
