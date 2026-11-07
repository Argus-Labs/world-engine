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

// startDeadListener binds an accept-and-close loop on addr, mimicking Docker's
// port proxy in front of a container that is not serving: a TCP dial succeeds
// but an HTTP request must not. This is what a crashed sibling looks like to
// WaitForShardsReady's probes. Returns the listener (close to stop) and the
// bound port (useful when addr is "127.0.0.1:0"). Shared with the probe tests so
// the two packages of readiness tests cannot drift on the listener contract.
func startDeadListener(t *testing.T, addr string) (net.Listener, int) {
	t.Helper()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("listen %s: %v", addr, err)
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

// startServingListener binds a real HTTP server on addr, so shardReady counts
// it as ready (it replies to an HTTP request, even with 404). Returns the server
// (close to stop) and the bound port.
func startServingListener(t *testing.T, addr string) (*http.Server, int) {
	t.Helper()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("listen %s: %v", addr, err)
	}
	srv := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.NotFoundHandler()}
	go func() { _ = srv.Serve(ln) }()
	return srv, ln.Addr().(*net.TCPAddr).Port
}

// poolShardConfig builds a 2-instance "game" pool for the reproduction tests.
// Ports are not in the config — the waiter derives them via shardHostPort, which
// the tests override to free ports so a running local world cannot collide.
func poolShardConfig() *service.Config {
	return &service.Config{Project: "demo", WorldToml: worldtoml.Config{
		Project: "demo",
		Shards: []worldtoml.Shard{
			{ID: "game", InstanceID: "game"},
			{ID: "game", InstanceID: "game-2"},
		},
	}}
}

// withFreePorts overrides shardHostPort to return the given free ports (one per
// shard instance, in world.toml order) for the duration of fn, then restores it.
// The listeners are bound up front on "127.0.0.1:0" and their actual ports feed
// shardHostPort, so the waiter probes real listeners without touching the
// production 8081+i ports a developer's running world may already hold.
func withFreePorts(t *testing.T, fn func(p0, p1 int)) {
	t.Helper()
	deadLn, p0 := startDeadListener(t, "127.0.0.1:0")
	defer deadLn.Close()
	srv, p1 := startServingListener(t, "127.0.0.1:0")
	defer srv.Close()

	prev := shardHostPort
	shardHostPort = func(i int) int { return []int{p0, p1}[i] }
	defer func() { shardHostPort = prev }()
	fn(p0, p1)
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
	withFreePorts(t, func(p0, p1 int) {
		r := New(nil, poolShardConfig())

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
	srv1, p0 := startServingListener(t, "127.0.0.1:0")
	defer srv1.Close()
	srv2, p1 := startServingListener(t, "127.0.0.1:0")
	defer srv2.Close()

	prev := shardHostPort
	shardHostPort = func(i int) int { return []int{p0, p1}[i] }
	defer func() { shardHostPort = prev }()

	r := New(nil, poolShardConfig())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	ready, expected := r.WaitForShardsReady(ctx, []string{"game", "game-2"}, nil)
	elapsed := time.Since(start)
	assert.Equal(t, 2, expected)
	assert.Equal(t, 2, ready)
	assert.Less(t, elapsed, 2*time.Second, "all ready should return well before the deadline")
}
