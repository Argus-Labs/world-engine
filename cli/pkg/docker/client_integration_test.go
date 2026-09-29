//go:build integration

package docker

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	"github.com/argus-labs/world-engine/cli/pkg/modfile"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// TestBuildCardinalImagesWithKo_BareBoneTemplate exercises the Cardinal image
// build path using the bare-bone template world project embedded in this
// repository. It uses ko to build a minimal shard and loads the resulting
// image into the local Docker daemon.
func TestBuildCardinalImagesWithKo_BareBoneTemplate(t *testing.T) {
	t.Log("This test requires a running Docker daemon, network access, and ko tooling.")

	// Resolve the test template directory relative to this file.
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	pkgDir := filepath.Dir(filename)
	templateDir := filepath.Join(pkgDir, "test", "bare-bone-template")

	// Skip if template directory doesn't exist
	info, err := os.Stat(templateDir)
	if err != nil || !info.IsDir() {
		t.Skipf("bare-bone-template directory not found at %s: %v (skipping test)", templateDir, err)
	}

	// Load the template world.toml.
	worldCfg, err := worldtoml.LoadFile(filepath.Join(templateDir, worldtoml.FileName))
	if err != nil {
		t.Fatalf("failed to load template world.toml: %v", err)
	}

	// Find the module root so ko builds from the Go module root rather than the
	// test directory.
	moduleRoot, ok := modfile.FindModuleRoot(pkgDir)
	if !ok {
		t.Skip("skipping Cardinal integration test; go.mod root not found")
	}

	// Adjust shard paths to point to the template shard package relative to the
	// module root so ko can import it.
	for i := range worldCfg.Shards {
		if worldCfg.Shards[i].Path == "" {
			worldCfg.Shards[i].Path = filepath.ToSlash(
				filepath.Join(
					"apps",
					"world-cli",
					"pkg",
					"docker",
					"test",
					"bare-bone-template",
					"shards",
					worldCfg.Shards[i].ID,
				),
			)
		} else {
			worldCfg.Shards[i].Path = filepath.ToSlash(
				filepath.Join(
					"apps",
					"world-cli",
					"pkg",
					"docker",
					"test",
					"bare-bone-template",
					worldCfg.Shards[i].Path,
				),
			)
		}
	}

	cfg := &service.Config{
		RootDir:   moduleRoot,
		Debug:     true,
		Namespace: "integration-bare-bone",
		NATSURL: "nats://" + service.DefaultNatsContainerName + ":" +
			strconv.Itoa(service.DefaultNatsClientPort),
		WorldToml: worldCfg,
	}

	client, err := NewClient(cfg, nil)
	if err != nil {
		t.Skipf("skipping Cardinal integration test; Docker not available: %v", err)
	}
	defer func() {
		_ = client.Close()
	}()

	// Build the Cardinal services from the adjusted world config.
	builders := service.BuildCardinalShards(cfg)
	cardinalServices := make([]service.Service, len(builders))
	for i, b := range builders {
		cardinalServices[i] = b(cfg)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if err := client.BuildCardinalImages(ctx, cardinalServices, nil); err != nil {
		t.Fatalf("BuildCardinalImages failed for bare-bone template: %v", err)
	}
}
