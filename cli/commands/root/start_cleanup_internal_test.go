package root

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
)

const injectedFailure = "injected failure"

// A start that fails before the log picker must stop the platform it brought up:
// NATS and Postgres restart unless stopped and keep their host ports. The shard
// build runs while the platform starts, so a failed build must stop it too.
func TestStartFailureStopsPlatform(t *testing.T) {
	const project = "demo"

	cases := []struct {
		name   string
		failOn string // "build", or the container whose create the daemon rejects
		starts int    // containers started before the failure
	}{
		{name: "shard build fails", failOn: "build", starts: 2},
		{name: "postgres fails after nats started", failOn: service.ProjectDBContainerName(project), starts: 1},
		{name: "shard create fails", failOn: service.CardinalShardContainerName(project, "game"), starts: 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			daemon := newStartDaemon(t, tc.failOn)

			dir := t.TempDir()
			worldToml := fmt.Sprintf("organization = \"o\"\nproject = %q\n\n[[shards]]\nid = \"game\"\n", project)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "world.toml"), []byte(worldToml), 0o600))

			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()

			err := (&StartCmd{Debug: true}).run(ctx, dir)
			require.ErrorContains(t, err, injectedFailure)

			assert.Equal(t, tc.starts, daemon.startCount(), "containers started before the failure")
			assert.Empty(t, daemon.runningContainers(), "world start failed but left containers running")
		})
	}
}

// startDaemon is a fake Docker Engine serving the calls world start makes. It
// tracks which containers run, so a test sees what a failed start leaves behind,
// and fails the test on any request it does not expect.
type startDaemon struct {
	t      *testing.T
	failOn string // "build", or the name of the container whose create is rejected

	mu      sync.Mutex
	labels  map[string]map[string]string // labels of each created container, by name
	running map[string]bool
	starts  int
}

func newStartDaemon(t *testing.T, failOn string) *startDaemon {
	t.Helper()

	d := &startDaemon{t: t, failOn: failOn, labels: map[string]map[string]string{}, running: map[string]bool{}}
	srv := httptest.NewServer(http.HandlerFunc(d.serve))
	t.Cleanup(srv.Close)
	t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(srv.URL, "http://"))
	t.Setenv("DOCKER_API_VERSION", "1.47")
	t.Setenv("DOCKER_TLS_VERIFY", "")
	t.Setenv("DOCKER_CERT_PATH", "")

	return d
}

func (d *startDaemon) startCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.starts
}

func (d *startDaemon) runningContainers() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	var names []string

	for name, on := range d.running {
		if on {
			names = append(names, name)
		}
	}

	return names
}

func (d *startDaemon) serve(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	// Paths arrive as /v1.47/<resource>/...; the ping may come unversioned.
	path := r.URL.Path
	if rest, ok := strings.CutPrefix(path, "/v"); ok {
		_, after, _ := strings.Cut(rest, "/")
		path = "/" + after
	}

	switch {
	case path == "/_ping":
		w.Header().Set("Api-Version", "1.47")
		_, _ = io.WriteString(w, "OK")
	case strings.HasPrefix(path, "/images/") && strings.HasSuffix(path, "/json"):
		// Every base image is already pulled.
		_, _ = io.WriteString(w, `{"Id":"sha256:base","Os":"linux","Architecture":"amd64"}`)
	case path == "/build":
		_, _ = io.Copy(io.Discard, r.Body)

		if d.failOn == "build" {
			_, _ = fmt.Fprintf(w, `{"error":%q}`, injectedFailure)

			return
		}

		_, _ = io.WriteString(w, `{}`)
	case strings.HasPrefix(path, "/networks/"):
		_, _ = io.WriteString(w, `{"Id":"net","Name":"net"}`)
	case path == "/containers/json":
		d.list(w)
	case path == "/containers/create":
		d.create(w, r)
	case strings.HasPrefix(path, "/containers/"):
		name, action, _ := strings.Cut(strings.TrimPrefix(path, "/containers/"), "/")
		d.container(w, r.Method+" "+action, name)
	default:
		d.t.Errorf("unexpected Docker request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (d *startDaemon) list(w http.ResponseWriter) {
	out := []map[string]any{}
	for name, labels := range d.labels {
		out = append(out, map[string]any{
			"Id": name, "Names": []string{"/" + name}, "Labels": labels, "State": d.state(name),
		})
	}

	_ = json.NewEncoder(w).Encode(out)
}

func (d *startDaemon) create(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == d.failOn {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, `{"message":%q}`, injectedFailure)

		return
	}

	var body struct {
		Labels map[string]string `json:"Labels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		d.t.Errorf("decode create body: %v", err)
	}

	d.labels[name] = body.Labels
	d.running[name] = false

	w.WriteHeader(http.StatusCreated)
	_, _ = fmt.Fprintf(w, `{"Id":%q,"Warnings":[]}`, name)
}

// container serves an inspect, start or stop of a container; IDs are names here.
func (d *startDaemon) container(w http.ResponseWriter, op, name string) {
	if _, ok := d.labels[name]; !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"No such container"}`)

		return
	}

	switch op {
	case "GET json":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Id":     name,
			"Config": map[string]any{"Labels": d.labels[name]},
			"State": map[string]any{
				"Status": d.state(name), "Running": d.running[name], "Health": map[string]any{"Status": "healthy"},
			},
		})
	case "POST start":
		d.running[name] = true
		d.starts++

		w.WriteHeader(http.StatusNoContent)
	case "POST stop":
		d.running[name] = false

		w.WriteHeader(http.StatusNoContent)
	default:
		d.t.Errorf("unexpected container request %s %s", op, name)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (d *startDaemon) state(name string) string {
	if d.running[name] {
		return "running"
	}

	return "exited"
}
