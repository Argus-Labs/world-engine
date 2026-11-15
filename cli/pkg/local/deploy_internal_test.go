package local

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
			daemon := newFakeDaemon(t, map[string]map[string]string{name: labels})

			cfg := &service.Config{Project: "demo", WorldToml: worldtoml.Config{
				Organization: "o",
				Project:      "demo",
				Shards:       []worldtoml.Shard{{ID: "game-api", InstanceID: "game-api"}},
			}}
			dc, err := docker.NewClient(cfg, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = dc.Close() })

			err = New(dc, cfg).Deploy(context.Background(), DeployOpts{Shards: []DeployShard{{ID: "game-api"}}})

			project, stopped := daemon.state(name)
			if tc.wantErr {
				assert.ErrorContains(t, err, tc.existingProject)
				assert.False(t, stopped, "Deploy stopped another project's container")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantProject, project)
		})
	}
}

// fakeDaemon serves the Docker Engine API calls Deploy makes, keeping each
// container's labels so a test can read back who owns a name.
type fakeDaemon struct {
	t          *testing.T
	mu         sync.Mutex
	containers map[string]map[string]string // name -> labels
	stopped    map[string]bool              // names a stop request was sent to
}

func newFakeDaemon(t *testing.T, containers map[string]map[string]string) *fakeDaemon {
	t.Helper()
	d := &fakeDaemon{t: t, containers: containers, stopped: map[string]bool{}}
	srv := httptest.NewServer(http.HandlerFunc(d.serve))
	t.Cleanup(srv.Close)
	t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(srv.URL, "http://"))
	t.Setenv("DOCKER_API_VERSION", "1.47")
	t.Setenv("DOCKER_TLS_VERIFY", "")
	t.Setenv("DOCKER_CERT_PATH", "")
	return d
}

// state returns the project label of the named container and whether the
// daemon was asked to stop it.
func (d *fakeDaemon) state(name string) (string, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.containers[name][service.ProjectLabel], d.stopped[name]
}

func (d *fakeDaemon) serve(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")

	// Paths arrive as /v1.47/containers/<name>/<action>.
	_, path, _ := strings.Cut(r.URL.Path, "/containers/")
	name, action, _ := strings.Cut(path, "/")
	switch {
	case r.Method == http.MethodGet && action == "json":
		labels, ok := d.containers[name]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"No such container"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Id":     name,
			"Name":   "/" + name,
			"Config": map[string]any{"Labels": labels},
			"State":  map[string]any{"Status": "exited"},
		})
	case r.Method == http.MethodPost && action == "stop":
		d.stopped[name] = true
		w.WriteHeader(http.StatusNotModified)
	case r.Method == http.MethodDelete && action == "":
		delete(d.containers, name)
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && name == "create":
		var body struct {
			Labels map[string]string `json:"Labels"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			d.t.Errorf("decode create body: %v", err)
		}
		d.containers[r.URL.Query().Get("name")] = body.Labels
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"Id":"created","Warnings":[]}`))
	case r.Method == http.MethodPost && action == "start":
		w.WriteHeader(http.StatusNoContent)
	default:
		d.t.Errorf("unexpected Docker request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}
}
