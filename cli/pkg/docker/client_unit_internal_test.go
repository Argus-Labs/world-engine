package docker

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/moby/moby/api/types/registry"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/argus-labs/world-engine/cli/pkg/dnslabel"
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
	if cfg.Namespace != dnslabel.Sanitize(filepath.Base(tmp)) {
		t.Fatalf("expected Namespace %q, got %q", dnslabel.Sanitize(filepath.Base(tmp)), cfg.Namespace)
	}

	if cfg.WorldToml.Organization != "argus" || cfg.WorldToml.Project != "rampage" {
		t.Fatalf("unexpected world.toml values: %+v", cfg.WorldToml)
	}

	expectedNATS := "nats://" + service.DefaultNatsContainerName + ":" +
		strconv.Itoa(service.DefaultNatsClientPort)
	if cfg.NATSURL != expectedNATS {
		t.Fatalf("expected NATSURL %q, got %q", expectedNATS, cfg.NATSURL)
	}
}

func TestNewClientConfig_MissingWorldToml(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	if _, err := NewClientConfig(tmp, false); err == nil {
		t.Fatalf("expected error when %s is missing", worldtoml.FileName)
	}
}

func TestNewClientConfig_SanitizesNamespace(t *testing.T) {
	t.Parallel()

	const sample = `
organization = "argus"
project = "rampage"
[[shards]]
id = "matchmaking"
`

	cases := []struct {
		name          string
		dirName       string
		wantNS        string
		wantImage     string
		wantContainer string
		shard         worldtoml.Shard
	}{
		{
			name:          "uppercase directory lowercased",
			dirName:       "MyGame",
			wantNS:        "mygame",
			wantImage:     "mygame-matchmaking-shard",
			wantContainer: "mygame-matchmaking-shard",
			shard:         worldtoml.Shard{ID: "matchmaking", InstanceID: "matchmaking", Path: "shards/matchmaking/"},
		},
		{
			name:          "spaces collapsed to hyphen",
			dirName:       "my game",
			wantNS:        "my-game",
			wantImage:     "my-game-matchmaking-shard",
			wantContainer: "my-game-matchmaking-shard",
			shard:         worldtoml.Shard{ID: "matchmaking", InstanceID: "matchmaking", Path: "shards/matchmaking/"},
		},
		{
			name:          "mixed case and underscores",
			dirName:       "My_Cool_Game",
			wantNS:        "my-cool-game",
			wantImage:     "my-cool-game-matchmaking-shard",
			wantContainer: "my-cool-game-matchmaking-shard",
			shard:         worldtoml.Shard{ID: "matchmaking", InstanceID: "matchmaking", Path: "shards/matchmaking/"},
		},
		{
			name:          "dots replaced with hyphens",
			dirName:       "my.game",
			wantNS:        "my-game",
			wantImage:     "my-game-matchmaking-shard",
			wantContainer: "my-game-matchmaking-shard",
			shard:         worldtoml.Shard{ID: "matchmaking", InstanceID: "matchmaking", Path: "shards/matchmaking/"},
		},
		{
			name:          "already canonical passthrough",
			dirName:       "my-game",
			wantNS:        "my-game",
			wantImage:     "my-game-matchmaking-shard",
			wantContainer: "my-game-matchmaking-shard",
			shard:         worldtoml.Shard{ID: "matchmaking", InstanceID: "matchmaking", Path: "shards/matchmaking/"},
		},
		{
			name:          "instance container name differs from shard image id",
			dirName:       "MyGame",
			wantNS:        "mygame",
			wantImage:     "mygame-matchmaking-shard",
			wantContainer: "mygame-matchmaking-2-shard",
			shard:         worldtoml.Shard{ID: "matchmaking", InstanceID: "matchmaking-2", Path: "shards/matchmaking/"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			projectDir := filepath.Join(root, tc.dirName)
			if err := os.Mkdir(projectDir, 0o755); err != nil {
				t.Fatalf("failed to create project dir: %v", err)
			}

			if err := os.WriteFile(filepath.Join(projectDir, worldtoml.FileName), []byte(sample), 0o600); err != nil {
				t.Fatalf("failed to write world.toml: %v", err)
			}

			cfg, err := NewClientConfig(projectDir, false)
			if err != nil {
				t.Fatalf("NewClientConfig returned error: %v", err)
			}

			if cfg.Namespace != tc.wantNS {
				t.Fatalf("expected Namespace %q, got %q", tc.wantNS, cfg.Namespace)
			}

			image := service.CardinalShardImageName(cfg.Namespace, tc.shard.ID)
			if image != tc.wantImage {
				t.Fatalf("expected image %q, got %q", tc.wantImage, image)
			}

			container := service.CardinalShardContainerName(cfg.Namespace, tc.shard.InstanceID)
			if container != tc.wantContainer {
				t.Fatalf("expected container %q, got %q", tc.wantContainer, container)
			}

			if !dnslabel.IsCanonical(image) {
				t.Fatalf("image tag %q is not canonical (lowercase DNS-label-safe)", image)
			}
			if !dnslabel.IsCanonical(container) {
				t.Fatalf("container name %q is not canonical (lowercase DNS-label-safe)", container)
			}
		})
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

	cardinalSvc := service.Service{
		Name: "cardinal-shard",
		Labels: map[string]string{
			service.CardinalNamespaceLabel: "ns",
		},
	}
	if !IsCardinalService(cardinalSvc) {
		t.Fatalf("expected IsCardinalService to return true when label is present")
	}

	nonCardinal := service.Service{Name: "nats"}
	if IsCardinalService(nonCardinal) {
		t.Fatalf("expected IsCardinalService to return false when label is absent")
	}
}

func TestCardinalBuildImageNamesDedupesInOrder(t *testing.T) {
	t.Parallel()

	services := []service.Service{
		{
			Name:  "world-game-shard",
			Image: "world-game-shard",
			Labels: map[string]string{
				service.CardinalNamespaceLabel: "world",
			},
		},
		{
			Name:  "world-game-2-shard",
			Image: "world-game-shard",
			Labels: map[string]string{
				service.CardinalNamespaceLabel: "world",
			},
		},
		{
			Name:  "world-chat-shard",
			Image: "world-chat-shard",
			Labels: map[string]string{
				service.CardinalNamespaceLabel: "world",
			},
		},
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
	// NATS has no Cardinal label, so BuildCardinalImages should be a no-op.
	natsSvc := service.NATS(&service.Config{})

	if err := c.BuildCardinalImages(context.Background(), []service.Service{natsSvc}, nil); err != nil {
		t.Fatalf("expected no error when there are no Cardinal services, got %v", err)
	}
}

func TestBuildCardinalImages_MissingShardPath(t *testing.T) {
	t.Parallel()

	c := &Client{cfg: &service.Config{RootDir: t.TempDir()}, logger: slog.New(slog.DiscardHandler)}

	cardinalSvc := service.Service{
		Name: "cardinal-shard",
		Labels: map[string]string{
			service.CardinalNamespaceLabel: "ns",
		},
		// BuildArgs intentionally missing SHARD_PATH.
		BuildArgs: map[string]string{},
	}

	if err := c.BuildCardinalImages(context.Background(), []service.Service{cardinalSvc}, nil); err == nil {
		t.Fatalf("expected error when Cardinal service is missing SHARD_PATH build arg")
	}
}

// Regression: a build stopped by a sibling's failure reported "returned success
// but image is not present: error during connect: <the sibling's error>".
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
