package service

import (
	"slices"
	"testing"
	"time"

	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

func TestGameServiceFromConfigImageKind(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Namespace: "rampage-backend",
		NATSURL:   "nats://world-engine-nats:4222",
		WorldToml: worldtoml.Config{Project: "rampage"},
	}
	svc := GameServiceFromConfig(cfg, worldtoml.GameService{
		ID:          "postgres",
		Image:       "postgres:16",
		Env:         map[string]string{"POSTGRES_USER": "meta", "POSTGRES_DB": "meta"},
		Ports:       []int{5432},
		Volume:      "/var/lib/postgresql/data",
		Healthcheck: []string{"CMD-SHELL", "pg_isready -U meta -d meta"},
	})

	if svc.Name != "rampage-postgres-service" {
		t.Fatalf("expected container name %q, got %q", "rampage-postgres-service", svc.Name)
	}
	if svc.Image != "postgres:16" {
		t.Fatalf("expected image %q, got %q", "postgres:16", svc.Image)
	}
	// All env merged into a single deduplicated slice sorted by key.
	wantEnv := []string{
		"CARDINAL_REGION=us-west1",
		"NATS_URL=nats://world-engine-nats:4222",
		"POSTGRES_DB=meta",
		"POSTGRES_USER=meta",
	}
	if !slices.Equal(svc.Env, wantEnv) {
		t.Fatalf("expected env %#v, got %#v", wantEnv, svc.Env)
	}
	// Named volume == container name, so purge's volume delete cleans it up.
	wantBinds := []string{"rampage-postgres-service:/var/lib/postgresql/data"}
	if !slices.Equal(svc.Binds, wantBinds) {
		t.Fatalf("expected binds %#v, got %#v", wantBinds, svc.Binds)
	}
	if svc.Labels[GameServiceLabel] != "rampage" {
		t.Fatalf("expected game service label %q, got %#v", "rampage", svc.Labels)
	}
	// Pulled kind must not carry the cardinal label or the build pipeline
	// errors on a missing SHARD_PATH.
	if _, ok := svc.Labels[CardinalNamespaceLabel]; ok {
		t.Fatalf("image-kind service must not carry the cardinal label, got %#v", svc.Labels)
	}
	if svc.BuildTarget != "" || svc.Dockerfile != "" {
		t.Fatalf("image-kind service must be pull-only, got target %q dockerfile %q", svc.BuildTarget, svc.Dockerfile)
	}
	hc := svc.Healthcheck
	if hc == nil || !slices.Equal(hc.Test, []string{"CMD-SHELL", "pg_isready -U meta -d meta"}) {
		t.Fatalf("expected healthcheck test, got %#v", hc)
	}
	if hc.Interval != 2*time.Second || hc.Timeout != 3*time.Second || hc.Retries != 15 {
		t.Fatalf("unexpected healthcheck timings: %#v", hc)
	}
}

func TestGameServiceFromConfigPathKind(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Namespace: "rampage-backend",
		NATSURL:   "nats://world-engine-nats:4222",
		WorldToml: worldtoml.Config{Project: "rampage"},
	}
	svc := GameServiceFromConfig(cfg, worldtoml.GameService{
		ID:   "meta",
		Path: "services/meta/cmd",
		Env: map[string]string{
			"DB_DSN": "postgres://meta:meta@rampage-postgres-service:5432/meta?sslmode=disable",
		},
	})

	if svc.Name != "rampage-meta-service" {
		t.Fatalf("expected container name %q, got %q", "rampage-meta-service", svc.Name)
	}
	// Built kind images are tagged with the container name.
	if svc.Image != "rampage-meta-service" {
		t.Fatalf("expected image %q, got %q", "rampage-meta-service", svc.Image)
	}
	if svc.BuildTarget != "runtime" {
		t.Fatalf("expected build target runtime, got %q", svc.BuildTarget)
	}
	if svc.BuildArgs["SHARD_PATH"] != "services/meta/cmd" || svc.BuildArgs["SOURCE_PATH"] != "." {
		t.Fatalf("unexpected build args: %#v", svc.BuildArgs)
	}
	// Built kind carries both labels: cardinal label rides the shard build +
	// image-prune pipeline, game service label drives start/stop/purge buckets.
	if svc.Labels[GameServiceLabel] != "rampage" {
		t.Fatalf("expected game service label %q, got %#v", "rampage", svc.Labels)
	}
	if svc.Labels[CardinalNamespaceLabel] != "rampage-backend" {
		t.Fatalf("expected cardinal label %q, got %#v", "rampage-backend", svc.Labels)
	}
	if !slices.Contains(
		svc.Env,
		"DB_DSN=postgres://meta:meta@rampage-postgres-service:5432/meta?sslmode=disable",
	) {
		t.Fatalf("expected user env appended, got %#v", svc.Env)
	}
	if svc.Healthcheck != nil {
		t.Fatalf("expected no healthcheck, got %#v", svc.Healthcheck)
	}
	if len(svc.Binds) != 0 {
		t.Fatalf("expected no binds, got %#v", svc.Binds)
	}
	if len(svc.Dependencies) != 2 {
		t.Fatalf("expected golang + base image dependencies, got %#v", svc.Dependencies)
	}
}

func TestGameServiceFromConfigInjectsConfigDBCreds(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Namespace: "rampage-backend",
		NATSURL:   "nats://world-engine-nats:4222",
		WorldToml: worldtoml.Config{Project: "rampage"},
	}
	// world.toml sets no POSTGRES_* env; world-cli must inject its defaults so
	// the config database initializes with creds matching the derived DSN.
	svc := GameServiceFromConfig(cfg, worldtoml.GameService{
		ID:       "postgres",
		Image:    "postgres:16",
		ConfigDB: true,
		Ports:    []int{5432},
	})

	wantEnv := []string{
		"CARDINAL_REGION=us-west1",
		"NATS_URL=nats://world-engine-nats:4222",
		"POSTGRES_DB=rampage",
		"POSTGRES_PASSWORD=postgres",
		"POSTGRES_USER=postgres",
	}
	if !slices.Equal(svc.Env, wantEnv) {
		t.Fatalf("expected env %#v, got %#v", wantEnv, svc.Env)
	}
}

func TestGameServiceFromConfigConfigDBCredsTakePrecedence(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Namespace: "rampage-backend",
		NATSURL:   "nats://world-engine-nats:4222",
		WorldToml: worldtoml.Config{Project: "rampage"},
	}
	// Even if world.toml tried to set its own POSTGRES_*, world-cli's injected
	// defaults replace them in the merged env, so there are no duplicate keys and
	// the world-cli creds win.
	svc := GameServiceFromConfig(cfg, worldtoml.GameService{
		ID:       "postgres",
		Image:    "postgres:16",
		ConfigDB: true,
		Env:      map[string]string{"POSTGRES_USER": "meta", "POSTGRES_DB": "meta"},
	})

	wantEnv := []string{
		"CARDINAL_REGION=us-west1",
		"NATS_URL=nats://world-engine-nats:4222",
		"POSTGRES_DB=rampage",
		"POSTGRES_PASSWORD=postgres",
		"POSTGRES_USER=postgres",
	}
	if !slices.Equal(svc.Env, wantEnv) {
		t.Fatalf("expected env %#v, got %#v", wantEnv, svc.Env)
	}
}

func TestGameServiceFromConfigInjectsDBDSN(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Namespace: "rampage-backend",
		NATSURL:   "nats://world-engine-nats:4222",
		WorldToml: worldtoml.Config{
			Project:  "rampage",
			Services: []worldtoml.GameService{{ID: "meta", Path: "services/meta/cmd", DB: true}},
		},
	}
	// db = true with no explicit env: world-cli injects the shared project DB DSN
	// as the uniform DB_DSN, so world.toml need not hand-wire it.
	svc := GameServiceFromConfig(cfg, cfg.WorldToml.Services[0])
	wantDSN := "DB_DSN=postgres://postgres:postgres@rampage-db:5432/rampage?sslmode=disable"
	if !slices.Contains(svc.Env, wantDSN) {
		t.Fatalf("expected injected %q, got %#v", wantDSN, svc.Env)
	}
}

func TestGameServiceFromConfigDBDSNOverridable(t *testing.T) {
	t.Parallel()

	meta := worldtoml.GameService{
		ID:   "meta",
		Path: "services/meta/cmd",
		DB:   true,
		Env:  map[string]string{"DB_DSN": "postgres://custom@host:5432/db"},
	}
	cfg := &Config{
		Namespace: "rampage-backend",
		NATSURL:   "nats://world-engine-nats:4222",
		WorldToml: worldtoml.Config{Project: "rampage", Services: []worldtoml.GameService{meta}},
	}
	// A world.toml DB_DSN wins: it's merged first and the injected default only
	// fills the key when absent, so there's a single DB_DSN with the user value.
	svc := GameServiceFromConfig(cfg, meta)
	if !slices.Contains(svc.Env, "DB_DSN=postgres://custom@host:5432/db") {
		t.Fatalf("expected the user override DSN to win, got %#v", svc.Env)
	}
	if slices.Contains(svc.Env, "DB_DSN=postgres://postgres:postgres@rampage-db:5432/rampage?sslmode=disable") {
		t.Fatalf("expected no injected default DSN when world.toml sets DB_DSN, got %#v", svc.Env)
	}
}

func TestProjectDBService(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		WorldToml: worldtoml.Config{
			Project:  "rampage",
			Services: []worldtoml.GameService{{ID: "meta", Path: "services/meta/cmd", DB: true}},
		},
	}
	svc := ProjectDBService(cfg)

	if svc.Name != "rampage-db" {
		t.Fatalf("expected container name %q, got %q", "rampage-db", svc.Name)
	}
	if svc.Image != "postgres:16" {
		t.Fatalf("expected image %q, got %q", "postgres:16", svc.Image)
	}
	wantEnv := []string{
		"POSTGRES_USER=postgres",
		"POSTGRES_PASSWORD=postgres",
		"POSTGRES_DB=rampage",
	}
	if !slices.Equal(svc.Env, wantEnv) {
		t.Fatalf("expected env %#v, got %#v", wantEnv, svc.Env)
	}
	if svc.Labels[GameServiceLabel] != "rampage" {
		t.Fatalf("expected game service label %q, got %#v", "rampage", svc.Labels)
	}
	// Auto-provisioned DB is pull-only — never rides the cardinal build pipeline.
	if _, ok := svc.Labels[CardinalNamespaceLabel]; ok {
		t.Fatalf("project db must not carry the cardinal label, got %#v", svc.Labels)
	}
	if svc.BuildTarget != "" {
		t.Fatalf("project db must be pull-only, got build target %q", svc.BuildTarget)
	}
	// Named volume == container name, so purge's volume delete cleans it up.
	if !slices.Equal(svc.Binds, []string{"rampage-db:/var/lib/postgresql/data"}) {
		t.Fatalf("expected named volume bind, got %#v", svc.Binds)
	}
	hc := svc.Healthcheck
	if hc == nil || !slices.Equal(hc.Test, []string{"CMD-SHELL", "pg_isready -U postgres -d rampage"}) {
		t.Fatalf("expected pg_isready healthcheck, got %#v", hc)
	}
}

func TestBuildGameServicesPrependsAutoProjectDB(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		WorldToml: worldtoml.Config{
			Project:  "rampage",
			Services: []worldtoml.GameService{{ID: "meta", Path: "services/meta/cmd", DB: true}},
		},
	}
	builders := BuildGameServices(cfg)
	if len(builders) != 2 {
		t.Fatalf("expected auto db + meta = 2 builders, got %d", len(builders))
	}
	// The shared project DB is first so it starts (and goes healthy) before meta.
	if name := builders[0](cfg).Name; name != "rampage-db" {
		t.Fatalf("expected first service %q (auto project db), got %q", "rampage-db", name)
	}
	if name := builders[1](cfg).Name; name != "rampage-meta-service" {
		t.Fatalf("expected second service %q, got %q", "rampage-meta-service", name)
	}
}

func TestBuildGameServicesPreservesDeclarationOrder(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		WorldToml: worldtoml.Config{
			Project: "rampage",
			Services: []worldtoml.GameService{
				{ID: "postgres", Image: "postgres:16"},
				{ID: "meta", Path: "services/meta/cmd"},
			},
		},
	}
	builders := BuildGameServices(cfg)
	if len(builders) != 2 {
		t.Fatalf("expected 2 builders, got %d", len(builders))
	}
	if name := builders[0](cfg).Name; name != "rampage-postgres-service" {
		t.Fatalf("expected first service %q, got %q", "rampage-postgres-service", name)
	}
	if name := builders[1](cfg).Name; name != "rampage-meta-service" {
		t.Fatalf("expected second service %q, got %q", "rampage-meta-service", name)
	}
}
