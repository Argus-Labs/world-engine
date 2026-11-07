package local

import (
	"context"
	"testing"
)

// A listener that accepts and immediately closes mimics Docker's port proxy in front of a
// container that is not serving yet: a TCP dial succeeds, an HTTP request must not.
func TestShardReadyRejectsAcceptOnlyListener(t *testing.T) {
	ln, port := startDeadListener(t, "127.0.0.1:0")
	defer ln.Close()
	if shardReady(context.Background(), port) {
		t.Fatal("accept-then-close must not count as ready")
	}
}

func TestShardReadyAcceptsRealServer(t *testing.T) {
	srv, port := startServingListener(t, "127.0.0.1:0")
	defer srv.Close()
	if !shardReady(context.Background(), port) {
		t.Fatal("a serving shard must count as ready even when it answers 404")
	}
}
