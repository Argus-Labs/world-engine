// Package edge is the local reverse proxy world start runs in-process: it routes
// localhost:8080/<org>/<project>/<instance>/... to that shard instance's host
// port, the same URL shape the chart's GKE HTTPRoute serves on a cluster.
package edge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/argus-labs/world-engine/cli/pkg/dnslabel"
)

// DefaultAddr is where world start listens; the chart's local example has no routing, so this is the only edge.
const DefaultAddr = "127.0.0.1:8080"

// routeKey carries the resolved backend from ServeHTTP to the shared proxy's Rewrite.
type routeKey struct{}

// Edge routes by the first three path segments and strips them, like GKE's ReplacePrefixMatch.
type Edge struct {
	routes atomic.Pointer[map[string]*url.URL] // RouteKey -> http://127.0.0.1:<port>
	proxy  *httputil.ReverseProxy
}

// New returns an Edge with an empty route table and an h2c transport to the shards.
func New() *Edge {
	t := &http.Transport{Protocols: new(http.Protocols)}
	t.Protocols.SetUnencryptedHTTP2(true) // leaving HTTP1 unset forces h2c, which is what shards speak
	e := &Edge{}
	e.SetRoutes(nil)
	// One proxy for every request: the backend and stripped path ride on the request
	// context, so nothing per-request has to be allocated on the game's hot path.
	e.proxy = &httputil.ReverseProxy{
		Transport:     t,
		FlushInterval: -1, // flush every write so server streams are not buffered
		Rewrite: func(pr *httputil.ProxyRequest) {
			route, ok := pr.In.Context().Value(routeKey{}).(*resolvedRoute)
			if !ok {
				return // only ServeHTTP reaches this proxy, and it always sets the route
			}
			pr.SetURL(route.backend)
			// RawPath is cleared deliberately: the stripped path comes from the decoded
			// URL, and ConnectRPC procedure paths carry no percent-encoding.
			pr.Out.URL.Path, pr.Out.URL.RawPath = route.path, ""
		},
		// A shard that is down is an expected state here (RefreshRoutes keeps a route
		// for every declared instance), so answer with something the developer can act
		// on instead of letting net/http log over the running TUI.
		ErrorLog: slog.NewLogLogger(slog.DiscardHandler, slog.LevelError),
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, _ error) {
			instance := "shard"
			if route, ok := r.Context().Value(routeKey{}).(*resolvedRoute); ok {
				instance = route.instance
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.WriteHeader(http.StatusBadGateway)
			// #nosec G705 -- instance is a key that already matched the route table, and the
			// response is text/plain with nosniff, so it cannot be interpreted as markup.
			fmt.Fprintf(w, "%s is not reachable; it may be starting or crashed — try `world logs`\n", instance)
		},
	}
	return e
}

// resolvedRoute is what ServeHTTP worked out for one request.
type resolvedRoute struct {
	backend  *url.URL
	path     string
	instance string
}

// RouteKey is "org/project/instance" with org and project sanitized the way the chart's pathSegment does.
func RouteKey(org, project, instance string) string {
	return dnslabel.Sanitize(org) + "/" + dnslabel.Sanitize(project) + "/" + instance
}

// SetRoutes swaps the table atomically; called after start and after each reload.
func (e *Edge) SetRoutes(r map[string]*url.URL) {
	if r == nil {
		r = map[string]*url.URL{}
	}
	e.routes.Store(&r)
}

// Routes returns a copy of the current table.
func (e *Edge) Routes() map[string]*url.URL {
	cur := *e.routes.Load()
	out := make(map[string]*url.URL, len(cur))
	maps.Copy(out, cur)
	return out
}

func (e *Edge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	seg := strings.SplitN(r.URL.Path, "/", 5) // "", org, project, instance, rest
	if len(seg) < 5 {
		http.NotFound(w, r)
		return
	}
	backend, ok := (*e.routes.Load())[seg[1]+"/"+seg[2]+"/"+seg[3]]
	if !ok {
		http.NotFound(w, r)
		return
	}
	route := &resolvedRoute{backend: backend, path: "/" + seg[4], instance: seg[3]}
	e.proxy.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), routeKey{}, route)))
}

// Serve listens on addr for HTTP/1.1 and h2c until ctx is cancelled. A bind
// failure names the port so the user can find the squatter.
func (e *Edge) Serve(ctx context.Context, addr string) error {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if err != nil {
		_, port, _ := net.SplitHostPort(addr)
		return fmt.Errorf(
			"edge cannot listen on %s (something else holds port %s; try `lsof -i :%s`): %w",
			addr,
			port,
			port,
			err,
		)
	}
	p := new(http.Protocols)
	p.SetHTTP1(true)
	p.SetUnencryptedHTTP2(true) // Unity and curl --http2-prior-knowledge send h2c directly
	srv := &http.Server{Handler: e, Protocols: p, ReadHeaderTimeout: 10 * time.Second}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
