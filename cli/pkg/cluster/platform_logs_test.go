package cluster

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/toml"
)

func TestProjectServicePods(t *testing.T) {
	cfg := toml.Config{
		Project: "rampage",
		Services: []toml.GameService{
			{ID: "meta", Path: "services/meta", DB: true},
			{ID: "match", Image: "ghcr.io/x/match:1"},
		},
	}

	// Services in declaration order (both path- and image-kind), then the auto
	// DB last — there's no config_db service, so DeployWorld provisions
	// "{project}-db" and the picker surfaces it. Selectors match the Deployment
	// "app" labels.
	want := []PlatformPodRef{
		{Name: "meta", Namespace: operatorNamespace, Selector: "app=rampage-meta-service"},
		{Name: "match", Namespace: operatorNamespace, Selector: "app=rampage-match-service"},
		{Name: "db", Namespace: operatorNamespace, Selector: "app=rampage-db"},
	}
	require.Equal(t, want, ProjectServicePods("rampage", cfg))
}

// TestProjectServicePods_ShardOnlyAutoDB covers the divergent case from PR #746:
// shards but no db=true service and no config_db service. DeployWorld provisions
// "{project}-db" (gated on !hasConfigDBService at client.go:163), so the picker
// must surface that auto DB — previously it was hidden by gating on the narrower
// needsAutoProjectDB.
func TestProjectServicePods_ShardOnlyAutoDB(t *testing.T) {
	cfg := toml.Config{
		Project: "rampage",
		Shards:  []toml.Shard{{ID: "gameplay"}},
	}
	require.Equal(t, []PlatformPodRef{
		{Name: "db", Namespace: operatorNamespace, Selector: "app=rampage-db"},
	}, ProjectServicePods("rampage", cfg))
}

// TestProjectServicePods_BareBoneTemplateAutoDB loads the canonical in-repo test
// fixture (the world-cli integration suite's test project) — shards and no
// [[services]] — and asserts the picker surfaces the auto "{project}-db".
// DeployWorld provisions it for this config, so the picker must list it rather
// than hide it. This is the exact config the bug report names as broken.
func TestProjectServicePods_BareBoneTemplateAutoDB(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "docker", "test", "bare-bone-template", "world.toml"))
	require.NoError(t, err)

	cfg, err := toml.Load(bytes.NewReader(b))
	require.NoError(t, err)
	require.Equal(t, "project", cfg.Project)

	require.Equal(t, []PlatformPodRef{
		{Name: "db", Namespace: operatorNamespace, Selector: "app=project-db"},
	}, ProjectServicePods(cfg.Project, cfg))
}

func TestProjectServicePods_ConfigDBSuppressesAutoDB(t *testing.T) {
	// A config_db service provides the DB, so no auto "{project}-db" is added —
	// only the declared service shows up.
	cfg := toml.Config{
		Project: "rampage",
		Services: []toml.GameService{
			{ID: "pg", Image: "postgres:16", ConfigDB: true},
		},
	}
	got := ProjectServicePods("rampage", cfg)
	require.Len(t, got, 1)
	require.Equal(t, "pg", got[0].Name)
}
