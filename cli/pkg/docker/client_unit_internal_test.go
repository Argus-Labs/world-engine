package docker

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/moby/moby/api/types/registry"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
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
	if cfg.Namespace != filepath.Base(tmp) {
		t.Fatalf("expected Namespace %q, got %q", filepath.Base(tmp), cfg.Namespace)
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
