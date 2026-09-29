package service

import (
	"slices"
	"testing"

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
			name: "db = true and no config_db -> auto-provision",
			cfg: worldtoml.Config{
				Project:  "rampage",
				Services: []worldtoml.GameService{{ID: "meta", Path: "x", DB: true}},
			},
			want: true,
		},
		{
			name: "db = true but explicit config_db satisfies it",
			cfg: worldtoml.Config{Project: "rampage", Services: []worldtoml.GameService{
				{ID: "postgres", Image: "postgres:16", ConfigDB: true},
				{ID: "meta", Path: "x", DB: true},
			}},
			want: false,
		},
		{
			name: "no db service",
			cfg:  worldtoml.Config{Project: "rampage", Services: []worldtoml.GameService{{ID: "meta", Path: "x"}}},
			want: false,
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

	cfg := &Config{Namespace: "world"}
	svc := CardinalFromShard(cfg, worldtoml.Shard{
		ID:         "game",
		InstanceID: "game-2",
		Path:       "shards/game/",
	}, DefaultCardinalDebugHostPort)

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
