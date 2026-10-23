package service

import (
	"slices"
	"testing"
	"time"

	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
	"github.com/argus-labs/world-engine/cli/pkg/version"
)

func TestGameServiceFromConfigImageKind(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Project:   "rampage",
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
	if svc.Labels[ProjectLabel] != "rampage" || svc.Labels[RoleLabel] != RoleService {
		t.Fatalf("expected project/service labels, got %#v", svc.Labels)
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
		Project:   "rampage",
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
	if svc.Labels[ProjectLabel] != "rampage" || svc.Labels[RoleLabel] != RoleService {
		t.Fatalf("expected project/service labels, got %#v", svc.Labels)
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
		Project:   "rampage",
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
		Project:   "rampage",
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
		Project: "rampage",
		NATSURL: "nats://world-engine-nats:4222",
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
		Project:   "rampage",
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
	if svc.Labels[ProjectLabel] != "rampage" || svc.Labels[RoleLabel] != RoleDB {
		t.Fatalf("expected project/db labels, got %#v", svc.Labels)
	}
	if svc.BuildTarget != "" {
		t.Fatalf("project db must be pull-only, got build target %q", svc.BuildTarget)
	}
	if !slices.Equal(svc.Binds, []string{"rampage-db-data:/var/lib/postgresql/data"}) {
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
	builders := BuildGameServices(cfg)[1:] // [0] is the auto project db
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

// PlatformImageRefs is the shared list both `world start` and the MCP
// `world_lifecycle` start pull before StartPlatform: NATS, the auto project DB
// when no config_db service brings its own, and every image-kind [[services]] entry.
// The tests pin exactly that set so the two paths cannot drift on which images a
// fresh machine needs.

func TestPlatformImageRefs_AutoProjectDBAndImageKindServices(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Project: "rampage",
		WorldToml: worldtoml.Config{
			Project: "rampage",
			Services: []worldtoml.GameService{
				{ID: "redis", Image: "redis:7"},         // image-kind: pulled
				{ID: "meta", Path: "services/meta/cmd"}, // source-built: skipped
			},
		},
	}
	// No config_db service, so the shared auto project DB applies (postgres:16).
	refs := PlatformImageRefs(cfg)
	want := []string{
		"nats:" + version.Nats, // NATS, always pulled
		"postgres:16",          // auto project DB (NeedsAutoProjectDB)
		"redis:7",              // image-kind [[services]] entry
	}
	if !slices.Equal(refs, want) {
		t.Fatalf("PlatformImageRefs = %v, want %v", refs, want)
	}
}

// A config_db [[services]] entry brings its own database, so NeedsAutoProjectDB is
// false: the auto "postgres:16" must NOT appear, while the config_db service's own
// image is listed (it is image-kind). A purge can't fix a missing image, so the MCP
// start surfaces the pull error plainly, but this list is what keeps it from happening.
func TestPlatformImageRefs_ConfigDBSuppressesAutoProjectDB(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Project: "rampage",
		WorldToml: worldtoml.Config{
			Project: "rampage",
			Services: []worldtoml.GameService{
				{ID: "postgres", Image: "postgres:14", ConfigDB: true}, // brings its own DB
				{ID: "meta", Path: "services/meta/cmd"},                // source-built: skipped
			},
		},
	}
	refs := PlatformImageRefs(cfg)
	// No auto project DB (postgres:16); the config_db service's own image is listed.
	want := []string{
		"nats:" + version.Nats,
		"postgres:14",
	}
	if !slices.Equal(refs, want) {
		t.Fatalf("PlatformImageRefs = %v, want %v", refs, want)
	}
	if slices.Contains(refs, "postgres:16") {
		t.Fatalf("auto project DB image must be absent when a config_db service exists, got %v", refs)
	}
}

// With no [[services]] at all and no config_db service, only NATS and the auto
// project DB need pulling — the minimal platform. This is the fresh-machine case
// the original bug hit: the pinned NATS image and postgres:16 are not dependencies
// of the shard build, so the build-deps pull does not fetch them.
func TestPlatformImageRefs_NATSAndAutoProjectDBOnly(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Project:   "rampage",
		WorldToml: worldtoml.Config{Project: "rampage"},
	}
	refs := PlatformImageRefs(cfg)
	want := []string{"nats:" + version.Nats, "postgres:16"}
	if !slices.Equal(refs, want) {
		t.Fatalf("PlatformImageRefs = %v, want %v", refs, want)
	}
}

// A source-built (path-kind) service is excluded from every case: it is built, and
// its build dependencies (golang + distroless base) are pulled by the build path, not
// here. Including it would try to pull a "<project>-<id>-service" name that has no tag.
func TestPlatformImageRefs_ExcludesSourceBuiltServices(t *testing.T) {
	t.Parallel()

	// A config_db service suppresses the auto DB, leaving only NATS plus any
	// image-kind services; with just a source-built service, that is NATS alone.
	cfg := &Config{
		Project: "rampage",
		WorldToml: worldtoml.Config{
			Project: "rampage",
			Services: []worldtoml.GameService{
				{ID: "db", Image: "postgres:16", ConfigDB: true}, // image-kind: included
				{ID: "meta", Path: "services/meta/cmd"},          // source-built: excluded
			},
		},
	}
	refs := PlatformImageRefs(cfg)
	if slices.Contains(refs, "rampage-meta-service") {
		t.Fatalf("source-built service image must not be pulled, got %v", refs)
	}
	if !slices.Equal(refs, []string{"nats:" + version.Nats, "postgres:16"}) {
		t.Fatalf("PlatformImageRefs = %v, want only NATS + the image-kind config_db service", refs)
	}
}
