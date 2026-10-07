package edge_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/cli/pkg/edge"
)

// h2cBackend mimics a shard: h2c-only, echoes the path it saw and streams two
// chunks, holding the second until release is closed (nil: write both at once).
func h2cBackend(t *testing.T, release <-chan struct{}) *url.URL {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := new(http.Protocols)
	p.SetUnencryptedHTTP2(true)
	srv := &http.Server{
		Protocols:         p,
		ReadHeaderTimeout: time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Proto", r.Proto)
			w.Header().Set("X-Path", r.URL.Path)
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "one;")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			if release != nil {
				<-release
			}
			fmt.Fprint(w, "two")
		}),
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return &url.URL{Scheme: "http", Host: ln.Addr().String()}
}

func newEdge(t *testing.T, release <-chan struct{}) (*edge.Edge, *httptest.Server) {
	t.Helper()
	e := edge.New()
	e.SetRoutes(map[string]*url.URL{edge.RouteKey("Argus Labs", "My_Game", "gameplay-2"): h2cBackend(t, release)})
	srv := httptest.NewUnstartedServer(e)
	p := new(http.Protocols)
	p.SetHTTP1(true)
	p.SetUnencryptedHTTP2(true)
	srv.Config.Protocols = p
	srv.Start()
	t.Cleanup(srv.Close)
	return e, srv
}

func TestEdgeStripsPrefixAndSpeaksH2CToShard(t *testing.T) {
	_, srv := newEdge(t, nil)
	resp, err := http.Get(srv.URL + "/argus-labs/my-game/gameplay-2/worldengine.cardinal.v1.CardinalService/Query")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "one;two" {
		t.Fatalf("status=%d body=%q", resp.StatusCode, body)
	}
	if got := resp.Header.Get("X-Path"); got != "/worldengine.cardinal.v1.CardinalService/Query" {
		t.Fatalf("backend saw path %q", got)
	}
	if got := resp.Header.Get("X-Proto"); got != "HTTP/2.0" {
		t.Fatalf("backend saw %s, want h2c", got)
	}
}

func TestEdgeAcceptsH2CFromClient(t *testing.T) {
	_, srv := newEdge(t, nil)
	tr := &http.Transport{Protocols: new(http.Protocols)}
	tr.Protocols.SetUnencryptedHTTP2(true)
	client := &http.Client{Transport: tr}
	resp, err := client.Get(srv.URL + "/argus-labs/my-game/gameplay-2/x")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Proto != "HTTP/2.0" || resp.StatusCode != http.StatusOK {
		t.Fatalf("proto=%s status=%d", resp.Proto, resp.StatusCode)
	}
}

func TestEdgeUnknownRouteIs404(t *testing.T) {
	_, srv := newEdge(t, nil)
	for _, path := range []string{"/nope", "/argus-labs/my-game/gameplay-2", "/argus-labs/my-game/lobby/x", "/"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s: status=%d, want 404", path, resp.StatusCode)
		}
	}
}

func TestEdgeStreamsFlushes(t *testing.T) {
	release := make(chan struct{})
	_, srv := newEdge(t, release)
	resp, err := http.Get(srv.URL + "/argus-labs/my-game/gameplay-2/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 16)
	n, err := resp.Body.Read(buf) // must return before the backend writes "two"
	if err != nil || string(buf[:n]) != "one;" {
		t.Fatalf("first read = %q, %v; want the first flushed chunk alone", buf[:n], err)
	}
	close(release)
	rest, _ := io.ReadAll(resp.Body)
	if string(rest) != "two" {
		t.Fatalf("rest = %q", rest)
	}
}

func TestEdgeRoutesSwapAtomically(t *testing.T) {
	e, srv := newEdge(t, nil)
	e.SetRoutes(nil)
	resp, err := http.Get(srv.URL + "/argus-labs/my-game/gameplay-2/x")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d after clearing routes", resp.StatusCode)
	}
}

// A declared instance whose container is down keeps its route, so the edge must
// answer with something the developer can act on rather than a bare 502.
func TestEdgeDownShardGetsActionable502(t *testing.T) {
	e := edge.New()
	dead, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := dead.Addr().String()
	_ = dead.Close() // nothing is listening there now
	e.SetRoutes(map[string]*url.URL{
		edge.RouteKey("argus", "demo", "gameplay-2"): {Scheme: "http", Host: addr},
	})
	srv := httptest.NewServer(e)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/argus/demo/gameplay-2/x")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
	if !strings.Contains(string(body), "gameplay-2") || !strings.Contains(string(body), "world logs") {
		t.Fatalf("body = %q, want the instance name and what to run", body)
	}
}

func TestServeReportsBusyPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	err = edge.New().Serve(context.Background(), ln.Addr().String())
	if err == nil || !strings.Contains(err.Error(), "lsof -i :") {
		t.Fatalf("err = %v, want a busy-port hint", err)
	}
}

func TestServeStopsOnContext(t *testing.T) {
	// Bind a port, release it, and serve on it, so the test can poll for the
	// listener instead of sleeping and racing the cancel against Listen.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := probe.Addr().String()
	_ = probe.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- edge.New().Serve(ctx, addr) }()
	for range 100 {
		if c, err := net.DialTimeout("tcp", addr, 50*time.Millisecond); err == nil {
			_ = c.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
}
