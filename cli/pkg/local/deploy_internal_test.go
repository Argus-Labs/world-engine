package local

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/docker"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// Project "demo" with shard "game-api" and project "demo-game" with shard "api"
// both name their shard container "demo-game-api-shard". Deploying "demo" must
// recreate its own container but leave the other project's alone.
func TestDeployRecreatesOnlyItsOwnProjectsContainer(t *testing.T) {
	name := service.CardinalShardContainerName("demo", "game-api")
	require.Equal(t, service.CardinalShardContainerName("demo-game", "api"), name)

	cases := map[string]struct {
		existingProject string // project label on the existing container; "" means none
		wantErr         bool
		wantProject     string // project label on the container after Deploy
	}{
		"own project's container is recreated": {
			existingProject: "demo",
			wantProject:     "demo",
		},
		// Containers from a CLI that predates the project label are still replaced.
		"unlabelled container is recreated": {
			existingProject: "",
			wantProject:     "demo",
		},
		"other project's container is kept": {
			existingProject: "demo-game",
			wantErr:         true,
			wantProject:     "demo-game",
		},
	}
	for desc, tc := range cases {
		t.Run(desc, func(t *testing.T) {
			labels := map[string]string{}
			if tc.existingProject != "" {
				labels[service.ProjectLabel] = tc.existingProject
			}

			daemon := newFakeDaemon(t)
			oldID := daemon.add(name, labels)

			err := deployDemo(t)

			current, ok := daemon.byName(name)
			require.True(t, ok, "no container named %s after Deploy", name)

			if tc.wantErr {
				assert.ErrorContains(t, err, tc.existingProject)
				assert.Empty(t, daemon.stopped, "Deploy stopped another project's container")
				assert.Equal(t, oldID, current.id, "Deploy replaced another project's container")
			} else {
				require.NoError(t, err)
				assert.Equal(t, []string{oldID}, daemon.stopped, "Deploy must stop the inspected container by ID")
				assert.Equal(t, []string{oldID}, daemon.removed, "Deploy must remove the inspected container by ID")
				assert.NotEqual(t, oldID, current.id, "Deploy did not recreate the container")
			}

			assert.Equal(t, tc.wantProject, current.labels[service.ProjectLabel])
		})
	}
}

// Another project's container can take the name between the ownership check and
// the stop. Deploy must stop and remove only the container it inspected, never the
// newcomer that now answers to the name.
func TestDeployKeepsContainerThatTakesTheNameAfterInspect(t *testing.T) {
	name := service.CardinalShardContainerName("demo", "game-api")

	daemon := newFakeDaemon(t)
	ownID := daemon.add(name, map[string]string{service.ProjectLabel: "demo"})

	var otherID string

	daemon.afterInspect = func() {
		daemon.afterInspect = nil
		daemon.delete(ownID)
		otherID = daemon.add(name, map[string]string{service.ProjectLabel: "demo-game"})
	}

	_ = deployDemo(t)

	assert.Equal(t, []string{ownID}, daemon.stopped, "Deploy must stop the inspected container by ID")
	assert.Equal(t, []string{ownID}, daemon.removed, "Deploy must remove the inspected container by ID")

	current, ok := daemon.byName(name)
	require.True(t, ok)
	assert.Equal(t, otherID, current.id, "Deploy replaced the container that took the name")
}

// A container removed by someone else after the inspect is already gone, so Deploy
// carries on and creates its own.
func TestDeployToleratesContainerGoneAfterInspect(t *testing.T) {
	name := service.CardinalShardContainerName("demo", "game-api")

	daemon := newFakeDaemon(t)
	ownID := daemon.add(name, map[string]string{service.ProjectLabel: "demo"})
	daemon.afterInspect = func() {
		daemon.afterInspect = nil
		daemon.delete(ownID)
	}

	require.NoError(t, deployDemo(t))

	current, ok := daemon.byName(name)
	require.True(t, ok, "Deploy did not create its container")
	assert.NotEqual(t, ownID, current.id)
	assert.Equal(t, "demo", current.labels[service.ProjectLabel])
}

// deployDemo deploys shard "game-api" of project "demo" against the fake daemon.
func deployDemo(t *testing.T) error {
	t.Helper()

	cfg := &service.Config{Project: "demo", WorldToml: worldtoml.Config{
		Organization: "o",
		Project:      "demo",
		Shards:       []worldtoml.Shard{{ID: "game-api", InstanceID: "game-api"}},
	}}
	dc, err := docker.NewClient(cfg, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dc.Close() })

	return New(dc, cfg).Deploy(context.Background(), DeployOpts{Shards: []DeployShard{{ID: "game-api"}}})
}

// fakeContainer is one container the fake daemon holds.
type fakeContainer struct {
	id     string
	name   string
	labels map[string]string
}

// fakeDaemon serves the Docker Engine API calls Deploy makes. Like Docker, it
// gives each container an ID distinct from its name and accepts either in paths.
type fakeDaemon struct {
	t            *testing.T
	mu           sync.Mutex
	nextID       int
	containers   []fakeContainer
	stopped      []string // the ID or name in each stop request
	removed      []string // the ID or name in each remove request
	afterInspect func()   // runs, under the lock, after each container inspect
}

func newFakeDaemon(t *testing.T) *fakeDaemon {
	t.Helper()

	d := &fakeDaemon{t: t}
	srv := httptest.NewServer(http.HandlerFunc(d.serve))
	t.Cleanup(srv.Close)
	t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(srv.URL, "http://"))
	t.Setenv("DOCKER_API_VERSION", "1.47")
	t.Setenv("DOCKER_TLS_VERIFY", "")
	t.Setenv("DOCKER_CERT_PATH", "")

	return d
}

// add stores a container and returns its ID. Callers outside serve must not hold the lock.
func (d *fakeDaemon) add(name string, labels map[string]string) string {
	d.nextID++
	id := fmt.Sprintf("%064x", d.nextID)
	d.containers = append(d.containers, fakeContainer{id: id, name: name, labels: labels})

	return id
}

func (d *fakeDaemon) delete(id string) {
	d.containers = slices.DeleteFunc(d.containers, func(c fakeContainer) bool { return c.id == id })
}

// find resolves a path reference, an ID or a name, as Docker does.
func (d *fakeDaemon) find(ref string) (fakeContainer, bool) {
	for _, c := range d.containers {
		if c.id == ref || c.name == ref {
			return c, true
		}
	}

	return fakeContainer{}, false
}

func (d *fakeDaemon) byName(name string) (fakeContainer, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, c := range d.containers {
		if c.name == name {
			return c, true
		}
	}

	return fakeContainer{}, false
}

func (d *fakeDaemon) notFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"message":"No such container"}`))
}

func (d *fakeDaemon) serve(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	// Paths arrive as /v1.47/containers/<id or name>/<action>.
	_, path, _ := strings.Cut(r.URL.Path, "/containers/")

	ref, action, _ := strings.Cut(path, "/")
	switch {
	case r.Method == http.MethodGet && action == "json":
		c, ok := d.find(ref)
		if !ok {
			d.notFound(w)

			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"Id":     c.id,
			"Name":   "/" + c.name,
			"Config": map[string]any{"Labels": c.labels},
			"State":  map[string]any{"Status": "exited"},
		})

		if d.afterInspect != nil {
			d.afterInspect()
		}
	case r.Method == http.MethodPost && action == "stop":
		d.stopped = append(d.stopped, ref)
		if _, ok := d.find(ref); !ok {
			d.notFound(w)

			return
		}

		w.WriteHeader(http.StatusNotModified)
	case r.Method == http.MethodDelete && action == "":
		d.removed = append(d.removed, ref)

		c, ok := d.find(ref)
		if !ok {
			d.notFound(w)

			return
		}

		d.delete(c.id)
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && ref == "create":
		var body struct {
			Labels map[string]string `json:"Labels"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			d.t.Errorf("decode create body: %v", err)
		}

		name := r.URL.Query().Get("name")
		if _, taken := d.find(name); taken {
			w.WriteHeader(http.StatusConflict)
			_, _ = fmt.Fprintf(w, `{"message":"Conflict. The container name %q is already in use"}`, "/"+name)

			return
		}

		id := d.add(name, body.Labels)

		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"Id":%q,"Warnings":[]}`, id)
	case r.Method == http.MethodPost && action == "start":
		w.WriteHeader(http.StatusNoContent)
	default:
		d.t.Errorf("unexpected Docker request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}
}
