package local

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

// A listener that accepts and immediately closes mimics Docker's port proxy in front of a
// container that is not serving yet: a TCP dial succeeds, an HTTP request must not.
func TestShardReadyRejectsAcceptOnlyListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	if shardReady(context.Background(), port) {
		t.Fatal("accept-then-close must not count as ready")
	}
}

func TestShardReadyAcceptsRealServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.NotFoundHandler()}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if !shardReady(context.Background(), port) {
		t.Fatal("a serving shard must count as ready even when it answers 404")
	}
}
