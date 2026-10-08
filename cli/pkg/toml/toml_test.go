package toml_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/toml"
)

func TestLoad_ParsesSampleConfig(t *testing.T) {
	t.Parallel()

	const sample = `# The user's forge organization.
	organization = "argus"

	# The project name.
	project = "rampage"

	# The shard ID injected into the container.
	[[shards]]
	id = "matchmaking"

	# Optional log level to override.
	[[shards]]
	id = "chat"
	log_level = "warn"

	# Optional path override.
	[[shards]]
	id = "game"
	path = "shards/rampage/"
	`

	cfg, err := toml.Load(strings.NewReader(sample))
	require.NoError(t, err, "Load should parse sample TOML without error.")

	require.Equal(t, "argus", cfg.Organization)
	require.Equal(t, "rampage", cfg.Project)
	require.Len(t, cfg.Shards, 3)

	// First shard: matchmaking.
	require.Equal(t, "matchmaking", cfg.Shards[0].ID)
	require.Equal(t, "info", cfg.Shards[0].LogLevel)
	require.Equal(t, "shards/matchmaking/", cfg.Shards[0].Path)

	// Second shard: chat with warn log level.
	require.Equal(t, "chat", cfg.Shards[1].ID)
	require.Equal(t, "warn", cfg.Shards[1].LogLevel)
	require.Equal(t, "shards/chat/", cfg.Shards[1].Path)

	// Third shard: game with path override.
	require.Equal(t, "game", cfg.Shards[2].ID)
	require.Equal(t, "info", cfg.Shards[2].LogLevel)
	require.Equal(t, "shards/rampage/", cfg.Shards[2].Path)
}

func TestLoad_UnknownKeys_Ignored(t *testing.T) {
	t.Parallel()

	const sampleWithUnknown = `
    organization = "argus"
    project = "rampage"
    unknown = 123
    [[shards]]
    id = "s1"
    `

	cfg, err := toml.Load(strings.NewReader(sampleWithUnknown))
	require.NoError(t, err)
	require.Equal(t, "argus", cfg.Organization)
	require.Equal(t, "rampage", cfg.Project)
	require.Len(t, cfg.Shards, 1)
	require.Equal(t, "s1", cfg.Shards[0].ID)
}

func TestLoad_MissingOrganization_ReturnsError(t *testing.T) {
	t.Parallel()

	const missingTopLevel = `
	[[shards]]
	id = "s1"
	`

	_, err := toml.Load(strings.NewReader(missingTopLevel))
	require.Error(t, err)
	// Organization is checked first, so we get that error when both are missing
	require.Contains(t, err.Error(), "world.toml is missing an Organization")
}

func TestLoad_MissingOnlyProject_ReturnsError(t *testing.T) {
	t.Parallel()

	const missingProject = `
	organization = "argus"
	[[shards]]
	id = "s1"
	`

	_, err := toml.Load(strings.NewReader(missingProject))
	require.Error(t, err)
	require.Contains(t, err.Error(), "world.toml is missing an Project")
	require.NotContains(t, err.Error(), "organization, project")
}

func TestLoad_NoShards_ReturnsError(t *testing.T) {
	t.Parallel()

	const noShards = `
	organization = "argus"
	project = "rampage"
	`

	_, err := toml.Load(strings.NewReader(noShards))
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one shard is required")
}

func TestLoad_ShardsMissingID_ReturnsError(t *testing.T) {
	t.Parallel()

	const shardMissingID = `
	organization = "argus"
	project = "rampage"
	[[shards]]
	log_level = "info"
	`

	_, err := toml.Load(strings.NewReader(shardMissingID))
	require.Error(t, err)
	require.Contains(t, err.Error(), "shards[0].id is required")
}

func TestLoadFile_ParsesSampleConfig(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"
	[[shards]]
	id = "matchmaking"
	[[shards]]
	id = "chat"
	log_level = "warn"
	[[shards]]
	id = "game"
	path = "shards/rampage/"
	`

	tmpDir := t.TempDir()
	path := tmpDir + "/config.toml"

	// Write the sample to a temp file.
	if err := os.WriteFile(path, []byte(sample), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := toml.LoadFile(path)
	require.NoError(t, err)
	require.Equal(t, "argus", cfg.Organization)
	require.Equal(t, "rampage", cfg.Project)
	require.Len(t, cfg.Shards, 3)
}

func TestLoad_DuplicateShardID_ReturnsErrorWithIndex(t *testing.T) {
	t.Parallel()

	const dup = `
	organization = "argus"
	project = "rampage"
	[[shards]]
	id = "s1"
	[[shards]]
	id = "s1"
	`

	_, err := toml.Load(strings.NewReader(dup))
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate shard id \"s1\" at index 1")
}

func TestLoad_OrganizationWithSpaces_SpacesRemoved(t *testing.T) {
	t.Parallel()

	const configWithSpaces = `
	organization = "argus labs"
	project = "rampage"
	[[shards]]
	id = "s1"
	`

	cfg, err := toml.Load(strings.NewReader(configWithSpaces))
	require.NoError(t, err)
	require.Equal(t, "arguslabs", cfg.Organization)
}

func TestLoad_ProjectWithSpecialCharacters_ReturnsError(t *testing.T) {
	t.Parallel()

	const configWithSpecialChars = `
	organization = "argus"
	project = "rampage@#$"
	[[shards]]
	id = "s1"
	`

	_, err := toml.Load(strings.NewReader(configWithSpecialChars))
	require.Error(t, err)
	require.Contains(t, err.Error(), "project contains invalid characters")
}

func TestLoad_ShardIDWithSpaces_SpacesRemoved(t *testing.T) {
	t.Parallel()

	const configWithShardSpaces = `
	organization = "argus"
	project = "rampage"
	[[shards]]
	id = "shard 1"
	`

	cfg, err := toml.Load(strings.NewReader(configWithShardSpaces))
	require.NoError(t, err)
	require.Equal(t, "shard1", cfg.Shards[0].ID)
}

func TestLoad_ValidConfigWithSpaces_SpacesRemoved(t *testing.T) {
	t.Parallel()

	const configWithSpaces = `
	organization = "argus labs"
	project = "rampage game"
	[[shards]]
	id = "shard 1"
	`

	cfg, err := toml.Load(strings.NewReader(configWithSpaces))
	require.NoError(t, err)
	require.Equal(t, "arguslabs", cfg.Organization)
	require.Equal(t, "rampagegame", cfg.Project)
	require.Equal(t, "shard1", cfg.Shards[0].ID)
}

func TestLoad_EmptyOrganizationAfterNormalization_ReturnsError(t *testing.T) {
	t.Parallel()

	const configWithEmptyOrg = `
	organization = "   "
	project = "rampage"
	[[shards]]
	id = "s1"
	`

	_, err := toml.Load(strings.NewReader(configWithEmptyOrg))
	require.Error(t, err)
	require.Contains(t, err.Error(), "organization cannot be empty after removing spaces")
}

func TestLoad_EmptyProjectAfterNormalization_ReturnsError(t *testing.T) {
	t.Parallel()

	const configWithEmptyProject = `
	organization = "argus"
	project = "!@#$%^&*()"
	[[shards]]
	id = "s1"
	`

	_, err := toml.Load(strings.NewReader(configWithEmptyProject))
	require.Error(t, err)
	require.Contains(t, err.Error(), "project contains invalid characters")
}

func TestLoad_EmptyShardIDAfterNormalization_ReturnsError(t *testing.T) {
	t.Parallel()

	const configWithEmptyShard = `
	organization = "argus"
	project = "rampage"
	[[shards]]
	id = "   "
	`

	_, err := toml.Load(strings.NewReader(configWithEmptyShard))
	require.Error(t, err)
	require.Contains(t, err.Error(), "shardID cannot be empty after removing spaces")
}

func TestLoad_PoolSizeDefaultsToOne_NoExpansion(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"
	[[shards]]
	id = "game"
	`

	cfg, err := toml.Load(strings.NewReader(sample))
	require.NoError(t, err)
	require.Len(t, cfg.Shards, 1)
	require.Equal(t, "game", cfg.Shards[0].ID)
	require.Equal(t, 1, cfg.Shards[0].PoolSize)
}

func TestLoad_PoolSizeExpandsIntoNumberedReplicas(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"
	[[shards]]
	id = "game"
	pool_size = 3
	[[shards]]
	id = "chat"
	`

	cfg, err := toml.Load(strings.NewReader(sample))
	require.NoError(t, err)
	require.Len(t, cfg.Shards, 4)

	require.Equal(t, "game", cfg.Shards[0].ID)
	require.Equal(t, "game", cfg.Shards[1].ID)
	require.Equal(t, "game", cfg.Shards[2].ID)
	require.Equal(t, "chat", cfg.Shards[3].ID)
	require.Equal(t, "game", cfg.Shards[0].InstanceID)
	require.Equal(t, "game-2", cfg.Shards[1].InstanceID)
	require.Equal(t, "game-3", cfg.Shards[2].InstanceID)
	require.Equal(t, "chat", cfg.Shards[3].InstanceID)

	// All replicas of "game" share the same source path.
	require.Equal(t, "shards/game/", cfg.Shards[0].Path)
	require.Equal(t, "shards/game/", cfg.Shards[1].Path)
	require.Equal(t, "shards/game/", cfg.Shards[2].Path)

	// Each expanded entry collapses to a single instance.
	for i := range cfg.Shards {
		require.Equal(t, 1, cfg.Shards[i].PoolSize, "shard %d", i)
	}
}

func TestLoad_PoolSizeExpansionInheritsLogLevelAndOTEL(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"
	[[shards]]
	id = "game"
	pool_size = 2
	log_level = "warn"
	enable_otel = true
	`

	cfg, err := toml.Load(strings.NewReader(sample))
	require.NoError(t, err)
	require.Len(t, cfg.Shards, 2)
	for _, s := range cfg.Shards {
		require.Equal(t, "warn", s.LogLevel)
		require.True(t, s.EnableOTEL)
	}
}

func TestLoad_PoolSizeExpansionCollidesWithExplicitID_ReturnsError(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"
	[[shards]]
	id = "game"
	pool_size = 2
	[[shards]]
	id = "game-2"
	`

	_, err := toml.Load(strings.NewReader(sample))
	require.Error(t, err)
	// Error should name both the colliding ID and the two source shards.
	require.Contains(t, err.Error(), `"game-2"`)
	require.Contains(t, err.Error(), `"game"`)
	require.Contains(t, err.Error(), "pool_size")
}

func TestLoad_PoolSizeExplicitZero_DefaultsToOne(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"
	[[shards]]
	id = "game"
	pool_size = 0
	`

	cfg, err := toml.Load(strings.NewReader(sample))
	require.NoError(t, err)
	require.Len(t, cfg.Shards, 1)
	require.Equal(t, "game", cfg.Shards[0].ID)
	require.Equal(t, 1, cfg.Shards[0].PoolSize)
}

func TestLoad_NegativePoolSize_DefaultsToOne(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"
	[[shards]]
	id = "game"
	pool_size = -1
	`

	cfg, err := toml.Load(strings.NewReader(sample))
	require.NoError(t, err)
	require.Len(t, cfg.Shards, 1)
	require.Equal(t, "game", cfg.Shards[0].ID)
	require.Equal(t, 1, cfg.Shards[0].PoolSize)
}

func TestLoad_ShardIDWithSpecialCharacters_ReturnsError(t *testing.T) {
	t.Parallel()

	const configWithShardSpecialChars = `
	organization = "argus"
	project = "rampage"
	[[shards]]
	id = "shard@#$"
	`

	_, err := toml.Load(strings.NewReader(configWithShardSpecialChars))
	require.Error(t, err)
	require.Contains(t, err.Error(), "shardID contains invalid characters")
}

func TestLoad_ParsesTickRateModeAndResources(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"
	tick_rate = 20
	mode = "LEADER"
	resources = { requests = { cpu = 250, memory = 256 }, limits = { cpu = 1000, memory = 1024 } }

	[[shards]]
	id = "meta"
	`

	cfg, err := toml.Load(strings.NewReader(sample))
	require.NoError(t, err)
	require.Len(t, cfg.Shards, 2)

	gameplay := cfg.Shards[0]
	require.Equal(t, "gameplay", gameplay.ID)
	require.Equal(t, int32(20), gameplay.TickRate)
	require.Equal(t, "LEADER", gameplay.Mode)
	require.NotNil(t, gameplay.Resources)
	require.Equal(t, int32(250), gameplay.Resources.Requests.CPU)
	require.Equal(t, int32(256), gameplay.Resources.Requests.Memory)
	require.Equal(t, int32(1000), gameplay.Resources.Limits.CPU)
	require.Equal(t, int32(1024), gameplay.Resources.Limits.Memory)

	// Second shard omits all of the new fields — defaults apply.
	meta := cfg.Shards[1]
	require.Equal(t, "meta", meta.ID)
	require.Equal(t, int32(0), meta.TickRate)
	require.Empty(t, meta.Mode)
	require.Nil(t, meta.Resources)
}

func TestLoad_PoolExpansionInheritsK8sFields(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"
	pool_size = 3
	tick_rate = 20
	mode = "LEADER"
	resources = { requests = { cpu = 250, memory = 256 }, limits = { cpu = 1000, memory = 1024 } }
	`

	cfg, err := toml.Load(strings.NewReader(sample))
	require.NoError(t, err)
	require.Len(t, cfg.Shards, 3)

	for i, s := range cfg.Shards {
		require.Equal(t, int32(20), s.TickRate, "shard %d", i)
		require.Equal(t, "LEADER", s.Mode, "shard %d", i)
		require.NotNil(t, s.Resources, "shard %d", i)
		require.Equal(t, int32(250), s.Resources.Requests.CPU, "shard %d", i)
		require.Equal(t, int32(1024), s.Resources.Limits.Memory, "shard %d", i)
	}
}

func TestLoad_NegativeTickRate_ReturnsError(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"
	tick_rate = -1
	`

	_, err := toml.Load(strings.NewReader(sample))
	require.Error(t, err)
	require.Contains(t, err.Error(), "tick_rate must be non-negative")
}

func TestLoad_NegativeResourceRequest_ReturnsError(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"
	resources = { requests = { cpu = -1, memory = 256 }, limits = { cpu = 1000, memory = 1024 } }
	`

	_, err := toml.Load(strings.NewReader(sample))
	require.Error(t, err)
	require.Contains(t, err.Error(), "resources.requests.cpu and .memory must each be >= 1")
}

func TestLoad_NegativeResourceLimit_ReturnsError(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"
	resources = { requests = { cpu = 250, memory = 256 }, limits = { cpu = 1000, memory = -1 } }
	`

	_, err := toml.Load(strings.NewReader(sample))
	require.Error(t, err)
	require.Contains(t, err.Error(), "resources.limits.cpu and .memory must each be >= 1")
}

func TestLoad_LimitBelowRequest_ReturnsError(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"
	resources = { requests = { cpu = 1000, memory = 256 }, limits = { cpu = 250, memory = 1024 } }
	`

	_, err := toml.Load(strings.NewReader(sample))
	require.Error(t, err)
	require.Contains(t, err.Error(), "limits.cpu (250) must be >= requests.cpu (1000)")
}

func TestLoad_MemoryLimitBelowRequest_ReturnsError(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"
	resources = { requests = { cpu = 250, memory = 2048 }, limits = { cpu = 1000, memory = 512 } }
	`

	_, err := toml.Load(strings.NewReader(sample))
	require.Error(t, err)
	require.Contains(t, err.Error(), "limits.memory (512) must be >= requests.memory (2048)")
}

// A resources block with requests but no limits is invalid: the CR encoder
// emits limits.{cpu,memory} as 0 (no omitempty) and the CRD requires them >= 1,
// so it would be rejected at apply. Validation must reject it up front and name
// the offending field.
func TestLoad_RequestsWithoutLimits_ReturnsError(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"
	resources = { requests = { cpu = 1000, memory = 2048 } }
	`

	_, err := toml.Load(strings.NewReader(sample))
	require.Error(t, err)
	require.Contains(t, err.Error(), "resources.limits.cpu and .memory must each be >= 1")
}

// Symmetrically, a resources block with limits but no requests is invalid:
// requests default to 0 and the CRD requires them >= 1.
func TestLoad_LimitsWithoutRequests_ReturnsError(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"
	resources = { limits = { cpu = 1000, memory = 2048 } }
	`

	_, err := toml.Load(strings.NewReader(sample))
	require.Error(t, err)
	require.Contains(t, err.Error(), "resources.requests.cpu and .memory must each be >= 1")
}

func TestLoad_ServiceImageKind_Parses(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"

	[[services]]
	id = "postgres"
	image = "postgres:16"
	env = { POSTGRES_USER = "meta", POSTGRES_PASSWORD = "meta", POSTGRES_DB = "meta" }
	ports = [5432]
	volume = "/var/lib/postgresql/data"
	healthcheck = ["CMD-SHELL", "pg_isready -U meta -d meta"]
	`

	cfg, err := toml.Load(strings.NewReader(sample))
	require.NoError(t, err)
	require.Len(t, cfg.Services, 1)

	svc := cfg.Services[0]
	require.Equal(t, "postgres", svc.ID)
	require.Equal(t, "postgres:16", svc.Image)
	require.Empty(t, svc.Path)
	require.Equal(t, map[string]string{
		"POSTGRES_USER":     "meta",
		"POSTGRES_PASSWORD": "meta",
		"POSTGRES_DB":       "meta",
	}, svc.Env)
	require.Equal(t, []int{5432}, svc.Ports)
	require.Equal(t, "/var/lib/postgresql/data", svc.Volume)
	require.Equal(t, []string{"CMD-SHELL", "pg_isready -U meta -d meta"}, svc.Healthcheck)
}

func TestLoad_ServicePathKind_Parses(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"

	[[services]]
	id = "meta"
	path = "services/meta/cmd"
	env = { DB_DSN = "postgres://meta:meta@rampage-postgres-service:5432/meta?sslmode=disable" }
	`

	cfg, err := toml.Load(strings.NewReader(sample))
	require.NoError(t, err)
	require.Len(t, cfg.Services, 1)

	svc := cfg.Services[0]
	require.Equal(t, "meta", svc.ID)
	require.Empty(t, svc.Image)
	require.Equal(t, "services/meta/cmd", svc.Path)
	require.Empty(t, svc.Ports)
	require.Empty(t, svc.Volume)
	require.Empty(t, svc.Healthcheck)
}

func TestLoad_ServiceMissingID_ReturnsError(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"

	[[services]]
	image = "postgres:16"
	`

	_, err := toml.Load(strings.NewReader(sample))
	require.Error(t, err)
	require.Contains(t, err.Error(), "services[0].id is required")
}

func TestLoad_ServiceImageAndPathBothSet_ReturnsError(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"

	[[services]]
	id = "meta"
	image = "postgres:16"
	path = "services/meta/cmd"
	`

	_, err := toml.Load(strings.NewReader(sample))
	require.Error(t, err)
	require.Contains(t, err.Error(), "must set exactly one of image or path")
}

func TestLoad_ServiceNeitherImageNorPath_ReturnsError(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"

	[[services]]
	id = "meta"
	`

	_, err := toml.Load(strings.NewReader(sample))
	require.Error(t, err)
	require.Contains(t, err.Error(), "must set exactly one of image or path")
}

func TestLoad_ServiceConfigDBAndDBBothSet_ReturnsError(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"

	[[services]]
	id = "postgres"
	image = "postgres:16"
	config_db = true
	db = true
	`

	_, err := toml.Load(strings.NewReader(sample))
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot set both config_db and db")
}

func TestLoad_DuplicateServiceID_ReturnsError(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"

	[[services]]
	id = "postgres"
	image = "postgres:16"

	[[services]]
	id = "postgres"
	image = "postgres:17"
	`

	_, err := toml.Load(strings.NewReader(sample))
	require.Error(t, err)
	require.Contains(t, err.Error(), `duplicate service id "postgres" at index 1`)
}

func TestLoad_ServiceTwoConfigDB_ReturnsError(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"

	[[services]]
	id = "postgres"
	image = "postgres:16"
	config_db = true

	[[services]]
	id = "postgres2"
	image = "postgres:16"
	config_db = true
	`

	_, err := toml.Load(strings.NewReader(sample))
	require.Error(t, err)
	require.Contains(t, err.Error(), "only one database per project")
}

func TestLoad_ServiceInvalidHealthcheck_ReturnsError(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"

	[[services]]
	id = "meta"
	image = "meta:latest"
	healthcheck = ["RUN", "true"]
	`

	_, err := toml.Load(strings.NewReader(sample))
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be CMD, CMD-SHELL, or NONE")
}

func TestLoad_ServiceConfigDBMultiPort_ReturnsError(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"

	[[services]]
	id = "postgres"
	image = "postgres:16"
	config_db = true
	ports = [8080, 5432]
	`

	_, err := toml.Load(strings.NewReader(sample))
	require.Error(t, err)
	require.Contains(t, err.Error(), "config_db must declare at most one port")
}

func TestLoad_ServiceConfigDBNon5432Port_ReturnsError(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"

	[[services]]
	id = "postgres"
	image = "postgres:16"
	config_db = true
	ports = [5433]
	`

	_, err := toml.Load(strings.NewReader(sample))
	require.Error(t, err)
	require.Contains(t, err.Error(), "config_db port must be 5432")
}

func TestLoad_ServiceConfigDBWithPath_ReturnsError(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"

	[[services]]
	id = "postgres"
	path = "services/postgres/"
	config_db = true
	`

	_, err := toml.Load(strings.NewReader(sample))
	require.Error(t, err)
	require.Contains(t, err.Error(), "config_db must use image, not path")
}

func TestLoad_ServicePortOutOfRange_ReturnsError(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"

	[[services]]
	id = "postgres"
	image = "postgres:16"
	ports = [70000]
	`

	_, err := toml.Load(strings.NewReader(sample))
	require.Error(t, err)
	require.Contains(t, err.Error(), "port 70000 must be between 1 and 65535")
}

func TestLoad_ServiceRelativeVolume_ReturnsError(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"

	[[services]]
	id = "postgres"
	image = "postgres:16"
	volume = "var/lib/postgresql/data"
	`

	_, err := toml.Load(strings.NewReader(sample))
	require.Error(t, err)
	require.Contains(t, err.Error(), "volume must be an absolute container path")
}

// limits == requests is a valid k8s config (guaranteed reservation == ceiling)
// and must be accepted — the exact boundary the `>=` comparison turns on, where
// an accidental `>` would wrongly reject it.
func TestLoad_LimitsEqualRequests_Allowed(t *testing.T) {
	t.Parallel()

	const sample = `
	organization = "argus"
	project = "rampage"

	[[shards]]
	id = "gameplay"
	resources = { requests = { cpu = 1000, memory = 1024 }, limits = { cpu = 1000, memory = 1024 } }
	`

	cfg, err := toml.Load(strings.NewReader(sample))
	require.NoError(t, err)
	require.NotNil(t, cfg.Shards[0].Resources)
	require.Equal(t, int32(1000), cfg.Shards[0].Resources.Limits.CPU)
	require.Equal(t, int32(1024), cfg.Shards[0].Resources.Limits.Memory)
}

// TestGameService_IsBuiltFromSource covers the docker-backend kind switch:
// Path set => built from source, Image set => pulled. validate guarantees
// exactly one is set, so the two branches are mutually exclusive.
func TestGameService_IsBuiltFromSource(t *testing.T) {
	t.Parallel()

	require.True(t, toml.GameService{Path: "services/meta/cmd"}.IsBuiltFromSource())
	require.False(t, toml.GameService{Image: "postgres:16"}.IsBuiltFromSource())
}

// worldTomlFor builds a minimal valid world.toml with one field set to value.
func worldTomlFor(field, value string) string {
	org, project, shardID, serviceID := "argus", "rampage", "gameplay", "meta"
	switch field {
	case "organization":
		org = value
	case "project":
		project = value
	case "shardID":
		shardID = value
	case "serviceID":
		serviceID = value
	}
	return fmt.Sprintf(`
	organization = %q
	project = %q
	[[shards]]
	id = %q
	[[services]]
	id = %q
	image = "postgres:16"
	`, org, project, shardID, serviceID)
}

func TestLoad_CanonicalNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		field, value string
		ok           bool
	}{
		{"project", "Rampage", false},
		{"project", "my_proj", false},
		{"project", "-game", false},
		{"shardID", "Game", false},
		{"serviceID", "meta_svc", false},
		{"organization", "My_Org", true},
	}
	for _, c := range cases {
		t.Run(c.field+"="+c.value, func(t *testing.T) {
			t.Parallel()
			_, err := toml.Load(strings.NewReader(worldTomlFor(c.field, c.value)))
			if c.ok {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, c.field+" contains invalid characters")
		})
	}
}
