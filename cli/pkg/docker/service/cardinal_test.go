package service

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/network"

	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

func TestConfigDBDSN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  worldtoml.Config
		want string
	}{
		{
			name: "no config_db service",
			cfg: worldtoml.Config{
				Project:  "rampage",
				Services: []worldtoml.GameService{{ID: "postgres", Image: "postgres:16"}},
			},
			want: "",
		},
		{
			name: "ignores service env, uses default creds + project-named db",
			cfg: worldtoml.Config{
				Project: "rampage",
				Services: []worldtoml.GameService{
					{
						ID:       "postgres",
						ConfigDB: true,
						Env: map[string]string{
							"POSTGRES_USER":     "meta",
							"POSTGRES_PASSWORD": "meta",
							"POSTGRES_DB":       "meta",
						},
						Ports: []int{5432},
					},
				},
			},
			want: "postgres://postgres:postgres@rampage-postgres-service:5432/rampage?sslmode=disable",
		},
		{
			name: "defaults to 5432 and project db when ports absent",
			cfg: worldtoml.Config{
				Project:  "game",
				Services: []worldtoml.GameService{{ID: "db", ConfigDB: true}},
			},
			want: "postgres://postgres:postgres@game-db-service:5432/game?sslmode=disable",
		},
		{
			// One config_db per project (enforced by validateServices); the DSN always
			// uses the in-network Postgres port (5432), not the declared host port. A
			// declared port is pinned to 5432 by validateServices anyway.
			name: "uses the in-network Postgres port, not the declared port",
			cfg: worldtoml.Config{
				Project: "game",
				Services: []worldtoml.GameService{
					{ID: "primary", ConfigDB: true, Ports: []int{5432}},
				},
			},
			want: "postgres://postgres:postgres@game-primary-service:5432/game?sslmode=disable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := configDBDSN(tt.cfg); got != tt.want {
				t.Fatalf("configDBDSN() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProjectDBDSN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  worldtoml.Config
		want string
	}{
		{
			name: "no database",
			cfg: worldtoml.Config{
				Project:  "rampage",
				Services: []worldtoml.GameService{{ID: "meta", Path: "services/meta/cmd"}},
			},
			want: "",
		},
		{
			name: "auto-provisioned project db for db = true",
			cfg: worldtoml.Config{
				Project:  "rampage",
				Services: []worldtoml.GameService{{ID: "meta", Path: "services/meta/cmd", DB: true}},
			},
			want: "postgres://postgres:postgres@rampage-db:5432/rampage?sslmode=disable",
		},
		{
			name: "explicit config_db service takes precedence over auto",
			cfg: worldtoml.Config{
				Project: "rampage",
				Services: []worldtoml.GameService{
					{ID: "postgres", Image: "postgres:16", ConfigDB: true, Ports: []int{5432}},
					{ID: "meta", Path: "services/meta/cmd", DB: true},
				},
			},
			want: "postgres://postgres:postgres@rampage-postgres-service:5432/rampage?sslmode=disable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := projectDBDSN(tt.cfg); got != tt.want {
				t.Fatalf("projectDBDSN() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNeedsAutoProjectDB(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  worldtoml.Config
		want bool
	}{
		{
			name: "no config_db -> auto-provision",
			cfg: worldtoml.Config{
				Project:  "rampage",
				Services: []worldtoml.GameService{{ID: "meta", Path: "x", DB: true}},
			},
			want: true,
		},
		{
			name: "explicit config_db replaces it",
			cfg: worldtoml.Config{Project: "rampage", Services: []worldtoml.GameService{
				{ID: "postgres", Image: "postgres:16", ConfigDB: true},
				{ID: "meta", Path: "x", DB: true},
			}},
			want: false,
		},
		{
			name: "shards alone still get a project db",
			cfg:  worldtoml.Config{Project: "rampage", Services: []worldtoml.GameService{{ID: "meta", Path: "x"}}},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NeedsAutoProjectDB(tt.cfg); got != tt.want {
				t.Fatalf("NeedsAutoProjectDB() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestBuildCardinalEnvSharesProjectDB locks that shards read the SAME DSN the
// db = true service does (one database per project) — the coupling that lets the
// meta service seed config the shards then load.
func TestBuildCardinalEnvSharesProjectDB(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		WorldToml: worldtoml.Config{
			Project:  "rampage",
			Services: []worldtoml.GameService{{ID: "meta", Path: "services/meta/cmd", DB: true}},
		},
	}
	env := buildCardinalEnv(cfg, worldtoml.Shard{ID: "gameplay", InstanceID: "gameplay"})
	if !slices.Contains(env, "DB_DSN=postgres://postgres:postgres@rampage-db:5432/rampage?sslmode=disable") {
		t.Fatalf("expected shard DB_DSN to point at the auto project db, got %#v", env)
	}
}

func TestCardinalFromShardUsesLogicalIDForImageAndInstanceIDForContainer(t *testing.T) {
	t.Parallel()

	cfg := &Config{Project: "world", WorldToml: worldtoml.Config{Project: "world"}}
	svc := CardinalFromShard(cfg, worldtoml.Shard{
		ID:         "game",
		InstanceID: "game-2",
		Path:       "shards/game/",
	}, ShardHostPortBase)

	if svc.Name != "world-game-2-shard" {
		t.Fatalf("expected container name %q, got %q", "world-game-2-shard", svc.Name)
	}
	if svc.Image != "world-game-shard" {
		t.Fatalf("expected shared image %q, got %q", "world-game-shard", svc.Image)
	}
	if !slices.Contains(svc.Env, "CARDINAL_SHARD_ID=game-2") {
		t.Fatalf("expected CARDINAL_SHARD_ID to use instance ID, got %#v", svc.Env)
	}
	// Cardinal shards build from source through the shared applySourceBuild;
	// lock the build fields so the byte-identity refactor stays honest.
	if svc.BuildTarget != "runtime" {
		t.Fatalf("expected build target runtime, got %q", svc.BuildTarget)
	}
	if svc.BuildArgs["SOURCE_PATH"] != "." || svc.BuildArgs["SHARD_PATH"] != "shards/game/" {
		t.Fatalf("unexpected build args: %#v", svc.BuildArgs)
	}
	if len(svc.Dependencies) != 2 {
		t.Fatalf("expected golang + base image dependencies, got %#v", svc.Dependencies)
	}
	if svc.Dependencies[0].Name != GoBuilderImage || svc.Dependencies[1].Name != BaseImage {
		t.Fatalf("unexpected dependency images: %#v", svc.Dependencies)
	}
}

// envMap turns KEY=VALUE pairs into a map, failing on duplicates.
func envMap(t *testing.T, env []string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		if _, dup := out[k]; dup {
			t.Fatalf("duplicate env %s", k)
		}
		out[k] = v
	}
	return out
}

// TestBuildCardinalEnvContract pins the local half of the chart contract (see
// cli/pkg/local contract test for the render comparison).
func TestBuildCardinalEnvContract(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Project: "rampage",
		NATSURL: NatsURL("rampage"),
		Debug:   true,
		WorldToml: worldtoml.Config{
			Organization: "argus",
			Project:      "rampage",
			Services:     []worldtoml.GameService{{ID: "meta", Path: "services/meta/cmd", DB: true}},
		},
	}
	got := envMap(t, buildCardinalEnv(cfg, worldtoml.Shard{ID: "gameplay", InstanceID: "gameplay-2", TickRate: 20}))
	want := map[string]string{
		"CARDINAL_SHARD_ID":              "gameplay-2",
		"CARDINAL_ORG":                   "argus",
		"CARDINAL_PROJECT":               "rampage",
		"CARDINAL_REGION":                "us-west1",
		"LOG_LEVEL":                      "info",
		"CARDINAL_SNAPSHOT_STORAGE_TYPE": "JETSTREAM",
		"CARDINAL_AUTH_MODE":             "DEV",
		"NATS_URL":                       "nats://rampage-nats:4222",
		"DB_DSN":                         "postgres://postgres:postgres@rampage-db:5432/rampage?sslmode=disable",
		"CARDINAL_TICK_RATE":             "20",
		"CARDINAL_MODE":                  "LEADER",
		"CARDINAL_DEBUG":                 "true",
		"OTEL_EXPORTER_OTLP_ENDPOINT":    "",
		"OTEL_RESOURCE_ATTRIBUTES":       "shard.id=gameplay,service.instance.id=gameplay-2",
		"LOG_FORMAT":                     "pretty",
	}
	if !maps.Equal(got, want) {
		t.Fatalf("env mismatch:\n got %#v\nwant %#v", got, want)
	}
}

func TestBuildCardinalEnvOmitsOptionalKeys(t *testing.T) {
	t.Parallel()

	cfg := &Config{Project: "g", NATSURL: NatsURL("g"), WorldToml: worldtoml.Config{Organization: "o", Project: "g"}}
	got := envMap(t, buildCardinalEnv(cfg, worldtoml.Shard{ID: "a", InstanceID: "a"}))
	for _, k := range []string{"CARDINAL_TICK_RATE", "OTEL_TRACE_SAMPLE_RATE"} {
		if _, ok := got[k]; ok {
			t.Fatalf("%s must be omitted when unset, got %#v", k, got)
		}
	}
	if got["CARDINAL_DEBUG"] != "false" || got["DB_DSN"] == "" || got["CARDINAL_MODE"] != "LEADER" {
		t.Fatalf("unexpected env %#v", got)
	}
	got = envMap(
		t,
		buildCardinalEnv(cfg, worldtoml.Shard{ID: "a", InstanceID: "a", Mode: "FOLLOWER", LogLevel: "debug"}),
	)
	if got["CARDINAL_MODE"] != "FOLLOWER" || got["LOG_LEVEL"] != "debug" {
		t.Fatalf("unexpected env %#v", got)
	}
}

func TestBuildCardinalShardsBindLoopbackPorts(t *testing.T) {
	t.Parallel()

	cfg := &Config{Project: "g", NATSURL: NatsURL("g"), WorldToml: worldtoml.Config{
		Organization: "o", Project: "g",
		Shards: []worldtoml.Shard{{ID: "gameplay", InstanceID: "gameplay"}, {ID: "gameplay", InstanceID: "gameplay-2"}},
	}}
	builders := BuildCardinalShards(cfg)
	if len(builders) != 2 {
		t.Fatalf("builders = %d", len(builders))
	}
	for i, want := range []string{"8081", "8082"} {
		svc := builders[i](cfg)
		b := svc.PortBindings[network.MustParsePort("8080/tcp")]
		if len(b) != 1 || b[0].HostPort != want || b[0].HostIP.String() != "127.0.0.1" {
			t.Fatalf("instance %d bindings = %#v", i, b)
		}
		if svc.NetworkMode != "g" || svc.RestartPolicy.Name != "" {
			t.Fatalf("instance %d host config = %#v", i, svc.HostConfig)
		}
		if svc.Labels[RoleLabel] != RoleShard || svc.Labels[ShardIDLabel] != "gameplay" ||
			svc.Labels[ProjectLabel] != "g" {
			t.Fatalf("instance %d labels = %#v", i, svc.Labels)
		}
	}
	if builders[1](cfg).Labels[InstanceLabel] != "gameplay-2" {
		t.Fatal("instance label")
	}
}

func TestNATSAndProjectDBArePerProject(t *testing.T) {
	t.Parallel()

	cfg := &Config{Project: "g", WorldToml: worldtoml.Config{Project: "g"}}
	n := NATS(cfg)
	if n.Name != "g-nats" || n.ReadyURL != "http://127.0.0.1:8222/healthz" || n.Healthcheck != nil ||
		n.NetworkMode != "g" {
		t.Fatalf("nats = %#v", n)
	}
	if !slices.Equal(n.Binds, []string{"g-nats-data:/data"}) || !slices.Contains(n.Cmd, "-js") {
		t.Fatalf("nats store = %#v %#v", n.Binds, n.Cmd)
	}
	db := ProjectDBService(cfg)
	if db.Name != "g-db" || db.Healthcheck == nil || db.NetworkMode != "g" || db.Labels[RoleLabel] != RoleDB {
		t.Fatalf("db = %#v", db)
	}
	if !slices.Equal(db.Binds, []string{"g-db-data:/var/lib/postgresql/data"}) {
		t.Fatalf("db binds = %#v", db.Binds)
	}
}

// TestBuildCardinalEnvArgusAuth locks the ARGUS pair: a local world can then be
// played with the same Argus accounts a hosted one uses.
func TestBuildCardinalEnvArgusAuth(t *testing.T) {
	t.Parallel()

	cfg := &Config{Project: "g", NATSURL: NatsURL("g"), WorldToml: worldtoml.Config{
		Organization: "o", Project: "g",
		Auth: worldtoml.Auth{Mode: worldtoml.AuthModeArgus, URL: "https://api.argus.dev"},
	}}
	got := envMap(t, buildCardinalEnv(cfg, worldtoml.Shard{ID: "a", InstanceID: "a"}))
	if got["CARDINAL_AUTH_MODE"] != "ARGUS" || got["CARDINAL_ARGUS_AUTH_URL"] != "https://api.argus.dev" {
		t.Fatalf("argus env = %#v", got)
	}
}

// An unparsed config (MCP reads by project name) must still produce a mode the shard accepts.
func TestBuildCardinalEnvDefaultsToDevAuth(t *testing.T) {
	t.Parallel()

	cfg := &Config{Project: "g", NATSURL: NatsURL("g"), WorldToml: worldtoml.Config{Organization: "o", Project: "g"}}
	got := envMap(t, buildCardinalEnv(cfg, worldtoml.Shard{ID: "a", InstanceID: "a"}))
	if got["CARDINAL_AUTH_MODE"] != "DEV" {
		t.Fatalf("auth mode = %q, want DEV", got["CARDINAL_AUTH_MODE"])
	}
	if _, ok := got["CARDINAL_ARGUS_AUTH_URL"]; ok {
		t.Fatalf("DEV must not carry an argus URL: %#v", got)
	}
}
