package docker

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/registry"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

func TestHasWorldImagePrefix(t *testing.T) {
	t.Parallel()

	prefixes := []string{
		"ghcr.io/argus/world-nats:",
		"ghcr.io/argus/world-registry:",
	}

	if !hasWorldImagePrefix("ghcr.io/argus/world-nats:v1", prefixes) {
		t.Fatalf("expected tag with matching prefix to return true")
	}

	if hasWorldImagePrefix("ubuntu:latest", prefixes) {
		t.Fatalf("expected tag without matching prefix to return false")
	}
}

func TestNewClientConfig_Success(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	const sample = `
organization = "argus"
project = "rampage"
[[shards]]
id = "matchmaking"
`

	if err := os.WriteFile(filepath.Join(tmp, worldtoml.FileName), []byte(sample), 0o600); err != nil {
		t.Fatalf("failed to write %s: %v", worldtoml.FileName, err)
	}

	cfg, err := NewClientConfig(tmp, true)
	if err != nil {
		t.Fatalf("NewClientConfig returned error: %v", err)
	}

	if cfg.RootDir != tmp {
		t.Fatalf("expected RootDir %q, got %q", tmp, cfg.RootDir)
	}
	if !cfg.Debug {
		t.Fatalf("expected Debug to be true")
	}
	if cfg.Project != "rampage" {
		t.Fatalf("expected Project %q, got %q", "rampage", cfg.Project)
	}

	if cfg.WorldToml.Organization != "argus" || cfg.WorldToml.Project != "rampage" {
		t.Fatalf("unexpected world.toml values: %+v", cfg.WorldToml)
	}

	if cfg.NATSURL != "nats://rampage-nats:4222" {
		t.Fatalf("unexpected NATSURL %q", cfg.NATSURL)
	}
}

func TestNewClientConfig_MissingWorldToml(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	if _, err := NewClientConfig(tmp, false); err == nil {
		t.Fatalf("expected error when %s is missing", worldtoml.FileName)
	}
}

// TestNewClientConfig_RelativePathAbsolutizesRootDir guards a bug where a relative
// projectDir was stored verbatim as cfg.RootDir and later resolved against the
// process's cwd. The MCP world_lifecycle and reload tools pass a caller-supplied
// worldPath through only TrimSpace + [filepath.Clean] (which keeps a relative path
// relative), so a world_path like "myworld" reached NewClientConfig relative.
// cfg.RootDir then fed the Docker build context (readDockerignore /
// archive.TarWithOptions) as a bare relative root, and stop/purge select
// containers by the project name read from world.toml — so a cwd-relative
// resolution could aim a destructive purge at the wrong world. NewClientConfig
// now absolutizes projectDir so RootDir is anchored regardless of where the
// long-lived MCP server process happens to be running.
func TestNewClientConfig_RelativePathAbsolutizesRootDir(t *testing.T) {
	// Not parallel: t.Chdir changes the process-wide cwd, which is shared state.
	wd := t.TempDir()

	// A subproject the caller references by a relative name. Its world.toml names a
	// DIFFERENT project than the one sitting in the cwd, so a resolution against the
	// wrong base — or a verbatim relative RootDir re-resolved later — would drift
	// the project name / build context.
	dir := filepath.Join(wd, "myworld")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	const subSample = `
organization = "argus"
project = "myworld"
[[shards]]
id = "game"
`
	if err := os.WriteFile(filepath.Join(dir, worldtoml.FileName), []byte(subSample), 0o600); err != nil {
		t.Fatalf("write subdir %s: %v", worldtoml.FileName, err)
	}
	const cwdSample = `
organization = "argus"
project = "cwdworld"
[[shards]]
id = "game"
`
	if err := os.WriteFile(filepath.Join(wd, worldtoml.FileName), []byte(cwdSample), 0o600); err != nil {
		t.Fatalf("write cwd %s: %v", worldtoml.FileName, err)
	}

	t.Chdir(wd)

	cfg, err := NewClientConfig("myworld", false)
	if err != nil {
		t.Fatalf("NewClientConfig returned error: %v", err)
	}

	// RootDir must be the absolutized, cwd-resolved path — not the verbatim
	// relative input — because that is the root the Docker build context is
	// tarred from.
	want := filepath.Join(wd, "myworld")
	if cfg.RootDir != want {
		t.Fatalf("expected RootDir absolutized to %q, got %q", want, cfg.RootDir)
	}
	if !filepath.IsAbs(cfg.RootDir) {
		t.Fatalf("expected absolute RootDir, got %q", cfg.RootDir)
	}
	// And it must load the targeted subdir's world.toml, not the cwd's.
	if cfg.Project != "myworld" {
		t.Fatalf("expected Project %q (from the targeted subdir), got %q", "myworld", cfg.Project)
	}
}

// TestNewClientConfig_DotPathAbsolutizesToCwd covers the "." case from the bug
// report: a bare "." resolves against the process cwd. Before the fix it was
// stored verbatim as RootDir=="." — leaving the build context root relative
// and unstable, and only coincidentally loading the cwd's world.toml.
func TestNewClientConfig_DotPathAbsolutizesToCwd(t *testing.T) {
	// Not parallel: t.Chdir changes the process-wide cwd, which is shared state.
	wd := t.TempDir()
	const sample = `
organization = "argus"
project = "cwdworld"
[[shards]]
id = "game"
`
	if err := os.WriteFile(filepath.Join(wd, worldtoml.FileName), []byte(sample), 0o600); err != nil {
		t.Fatalf("write %s: %v", worldtoml.FileName, err)
	}

	t.Chdir(wd)

	cfg, err := NewClientConfig(".", false)
	if err != nil {
		t.Fatalf("NewClientConfig returned error: %v", err)
	}
	if cfg.RootDir != wd {
		t.Fatalf("expected RootDir %q (the cwd), got %q", wd, cfg.RootDir)
	}
	if !filepath.IsAbs(cfg.RootDir) {
		t.Fatalf("expected absolute RootDir, got %q", cfg.RootDir)
	}
}

// TestNewClientConfig_RelativeMissingWorldToml_ErrorNamesAbsPath confirms the
// "Cannot find world.toml" error now points at the absolutized directory rather
// than the bare relative input. The bug report called out that the relative
// form gave "no hint of the wrong base directory"; the absolute path makes the
// resolution base diagnosable for the caller.
func TestNewClientConfig_RelativeMissingWorldToml_ErrorNamesAbsPath(t *testing.T) {
	// Not parallel: t.Chdir changes the process-wide cwd, which is shared state.
	wd := t.TempDir()
	missing := filepath.Join(wd, "ghost")
	if err := os.Mkdir(missing, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	t.Chdir(wd)

	_, err := NewClientConfig("ghost", false)
	if err == nil {
		t.Fatalf("expected error when %s is missing", worldtoml.FileName)
	}
	abs := filepath.Join(wd, "ghost")
	if !strings.Contains(err.Error(), abs) {
		t.Fatalf("expected error to name the absolutized path %q; got %q", abs, err.Error())
	}
}

func TestPreparePullOptions_NoAuth(t *testing.T) {
	t.Parallel()

	c := &Client{cfg: &service.Config{}, logger: slog.New(slog.DiscardHandler)}
	opts, err := c.preparePullOptions(ocispec.Platform{OS: "linux", Architecture: "amd64"}, nil)
	if err != nil {
		t.Fatalf("preparePullOptions returned error: %v", err)
	}
	if len(opts.Platforms) != 1 || opts.Platforms[0].OS != "linux" || opts.Platforms[0].Architecture != "amd64" {
		t.Fatalf("expected platform linux/amd64, got %+v", opts.Platforms)
	}
	if opts.RegistryAuth != "" {
		t.Fatalf("expected empty RegistryAuth when authConfig is nil")
	}
}

func TestPreparePullOptions_WithAuth(t *testing.T) {
	t.Parallel()

	c := &Client{cfg: &service.Config{}, logger: slog.New(slog.DiscardHandler)}
	auth := &registry.AuthConfig{
		Username: "user",
		Password: "pass",
	}

	opts, err := c.preparePullOptions(ocispec.Platform{}, auth)
	if err != nil {
		t.Fatalf("preparePullOptions with auth returned error: %v", err)
	}
	if opts.RegistryAuth == "" {
		t.Fatalf("expected non-empty RegistryAuth when authConfig is provided")
	}
}

func TestCheckPullEventError(t *testing.T) {
	t.Parallel()

	eventWithDetail := pullEvent{
		ErrorDetail: &pullErrorDetail{Message: "detail-error"},
	}
	if err := checkPullEventError(eventWithDetail, "img"); err == nil {
		t.Fatalf("expected checkPullEventError to return error for errorDetail")
	}

	eventWithError := pullEvent{
		Error: "plain-error",
	}
	if err := checkPullEventError(eventWithError, "img"); err == nil {
		t.Fatalf("expected checkPullEventError to return error for error")
	}

	// No error fields should return nil.
	okEvent := pullEvent{
		Status: "downloading",
	}
	if err := checkPullEventError(okEvent, "img"); err != nil {
		t.Fatalf("expected checkPullEventError to return nil when no error fields present, got %v", err)
	}
}

func TestExtractPullPercent(t *testing.T) {
	t.Parallel()

	event := pullEvent{
		ProgressDetail: &pullProgress{
			Current: 50,
			Total:   100,
		},
	}

	percent, ok := extractPullPercent(event)
	if !ok {
		t.Fatalf("expected extractPullPercent to return ok=true")
	}
	if percent != 50 {
		t.Fatalf("expected percent to be 50, got %d", percent)
	}

	// Event without progressDetail should return ok=false.
	eventNoProgress := pullEvent{
		Status: "done",
	}
	_, ok = extractPullPercent(eventNoProgress)
	if ok {
		t.Fatalf("expected extractPullPercent to return ok=false for event without progressDetail")
	}
}

func TestIsCardinalService(t *testing.T) {
	t.Parallel()

	// A real shard, as CardinalFromShard builds it: the build pipeline selects on the
	// build target, so the fixture has to carry one.
	cfg := &service.Config{Project: "demo", WorldToml: worldtoml.Config{Project: "demo"}}
	shard := service.CardinalFromShard(cfg, worldtoml.Shard{ID: "game", InstanceID: "game", Path: "shards/game"}, 8081)
	if !IsCardinalService(shard) {
		t.Fatalf("expected a shard to be built: %#v", shard)
	}

	if IsCardinalService(service.NATS(cfg)) {
		t.Fatal("expected a pulled image not to be built")
	}
}

func TestCardinalBuildImageNamesDedupesInOrder(t *testing.T) {
	t.Parallel()

	services := []service.Service{
		{Name: "world-game-shard", Image: "world-game-shard", BuildTarget: "runtime"},
		{Name: "world-game-2-shard", Image: "world-game-shard", BuildTarget: "runtime"},
		{Name: "world-chat-shard", Image: "world-chat-shard", BuildTarget: "runtime"},
		service.NATS(&service.Config{}),
	}

	names := CardinalBuildImageNames(services)
	want := []string{"world-game-shard", "world-chat-shard"}
	if len(names) != len(want) {
		t.Fatalf("expected %d image names, got %d: %#v", len(want), len(names), names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("expected image %d to be %q, got %q", i, want[i], names[i])
		}
	}
}

func TestBuildCardinalImages_NoCardinalServices(t *testing.T) {
	t.Parallel()

	c := &Client{cfg: &service.Config{RootDir: t.TempDir()}, logger: slog.New(slog.DiscardHandler)}
	// NATS is a pulled image, so BuildCardinalImages should be a no-op.
	natsSvc := service.NATS(&service.Config{})

	if err := c.BuildCardinalImages(context.Background(), []service.Service{natsSvc}, nil); err != nil {
		t.Fatalf("expected no error when there are no Cardinal services, got %v", err)
	}
}

func TestBuildCardinalImages_MissingShardPath(t *testing.T) {
	t.Parallel()

	c := &Client{cfg: &service.Config{RootDir: t.TempDir()}, logger: slog.New(slog.DiscardHandler)}

	cardinalSvc := service.Service{
		Name:        "cardinal-shard",
		BuildTarget: "runtime",
		// BuildArgs intentionally missing SHARD_PATH.
		BuildArgs: map[string]string{},
	}

	if err := c.BuildCardinalImages(context.Background(), []service.Service{cardinalSvc}, nil); err == nil {
		t.Fatalf("expected error when Cardinal service is missing SHARD_PATH build arg")
	}
}

// A path-kind [[services]] entry builds from project source, so the build pipeline must
// pick it up. Selecting by role instead silently skipped it and `world start` then tried
// to pull an image that only ever exists locally.
func TestIsCardinalService_IncludesPathKindGameServices(t *testing.T) {
	t.Parallel()

	cfg := &service.Config{Project: "demo", WorldToml: worldtoml.Config{Project: "demo"}}
	built := service.GameServiceFromConfig(cfg, worldtoml.GameService{ID: "meta", Path: "services/meta"})
	pulled := service.GameServiceFromConfig(cfg, worldtoml.GameService{ID: "db", Image: "postgres:16"})

	if !IsCardinalService(built) {
		t.Fatalf("path-kind service must be built: %#v", built)
	}
	if IsCardinalService(pulled) {
		t.Fatalf("image-kind service must not be built: %#v", pulled)
	}
	if names := CardinalBuildImageNames(
		[]service.Service{built, pulled},
	); len(names) != 1 ||
		names[0] != "demo-meta-service" {
		t.Fatalf("build image names = %v, want just the path-kind service", names)
	}
}

// The project label arrived after the image label, so purge has to recognise images an
// older CLI built — by name — without ever matching another project's.
func TestOwnsBuiltImage(t *testing.T) {
	t.Parallel()

	owned := map[string]struct{}{"demo-game-shard": {}, "demo-meta-service": {}}

	cases := map[string]struct {
		labels map[string]string
		want   bool
	}{
		"this project, labelled": {
			map[string]string{service.ProjectLabel: "demo", service.CardinalImageNameLabel: "demo-game-shard"}, true,
		},
		"another project, labelled": {
			map[string]string{service.ProjectLabel: "other", service.CardinalImageNameLabel: "other-game-shard"}, false,
		},
		"legacy image of this project": {
			map[string]string{service.CardinalImageNameLabel: "demo-game-shard"}, true,
		},
		"legacy path-kind service of this project": {
			map[string]string{service.CardinalImageNameLabel: "demo-meta-service"}, true,
		},
		"legacy image of another project": {
			map[string]string{service.CardinalImageNameLabel: "other-game-shard"}, false,
		},
		"no labels at all": {map[string]string{}, false},
		// A labelled image wins on the label even when the name happens to collide.
		"another project whose image name we also build": {
			map[string]string{service.ProjectLabel: "other", service.CardinalImageNameLabel: "demo-game-shard"}, false,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := ownsBuiltImage(tc.labels, "demo", owned); got != tc.want {
				t.Fatalf("ownsBuiltImage(%v) = %v, want %v", tc.labels, got, tc.want)
			}
		})
	}
}

// Build and pull fan-outs classify a sibling's cancellation fallout as canceled, not as
// the item's own failure.
func TestItemErrReportsFalloutAsCanceled(t *testing.T) {
	t.Parallel()

	own := errors.New("exit code: 1")
	gctx, cancel := context.WithCancelCause(context.Background())
	if got := itemErr(gctx, own); !errors.Is(got, own) {
		t.Fatalf("live group: want the item's own error, got %v", got)
	}

	cancel(errors.New("sibling failed"))
	if got := itemErr(gctx, own); !errors.Is(got, context.Canceled) {
		t.Fatalf("canceled group: want context.Canceled, got %v", got)
	}
}
