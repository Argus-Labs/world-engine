package service

import (
	"fmt"
	"strconv"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"

	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

const (
	// DefaultCardinalDebugPort is the Cardinal debug server port.
	DefaultCardinalDebugPort = 8080
	// DefaultCardinalDebugHostPort is the host port for Cardinal debug.
	DefaultCardinalDebugHostPort = 8081

	// DBDSNEnvVar is the uniform env var carrying the shared project database DSN
	// to every shard and every db = true service.
	DBDSNEnvVar = "DB_DSN"

	// projectDBImage is the Postgres image auto-provisioned for the shared
	// project database.
	projectDBImage = "postgres:16"

	// defaultConfigDBPort is the Postgres port assumed when a config_db service
	// declares none.
	defaultConfigDBPort = 5432

	// configDBUser/configDBPassword are world-cli's default credentials for the
	// shared project database; world.toml never sets POSTGRES_* itself.
	configDBUser     = "postgres"
	configDBPassword = "postgres"
)

func BuildCardinalShards(cfg *Config) []Builder {
	services := make([]Builder, 0, len(cfg.WorldToml.Shards))
	for i, sc := range cfg.WorldToml.Shards {
		shardCfg := sc
		hostPort := DefaultCardinalDebugHostPort + i
		services = append(services, func(c *Config) Service {
			return CardinalFromShard(c, shardCfg, hostPort)
		})
	}
	return services
}

// CardinalShardContainerName returns the per-instance container name for a
// Cardinal shard (e.g. "game-2").
func CardinalShardContainerName(namespace, instanceID string) string {
	return fmt.Sprintf("%s-%s-shard", namespace, instanceID)
}

// CardinalShardImageName returns the image tag shared by a Cardinal shard pool,
// keyed by the world.toml shard ID.
func CardinalShardImageName(namespace, shardID string) string {
	return fmt.Sprintf("%s-%s-shard", namespace, shardID)
}

func CardinalFromShard(cfg *Config, shard worldtoml.Shard, hostPort int) Service {
	containerName := CardinalShardContainerName(cfg.Namespace, shard.InstanceID)
	imageName := CardinalShardImageName(cfg.Namespace, shard.ID)

	env := buildCardinalEnv(cfg, shard)
	exposedPorts := []int{DefaultCardinalDebugPort}
	tcp := network.MustParsePort(strconv.Itoa(DefaultCardinalDebugPort) + "/tcp")
	portBindings := network.PortMap{tcp: []network.PortBinding{{HostPort: strconv.Itoa(hostPort)}}}

	svc := Service{
		Name: containerName,
		Config: container.Config{
			Image:        imageName,
			Env:          env,
			ExposedPorts: getExposedPorts(exposedPorts),
			Labels:       map[string]string{CardinalNamespaceLabel: cfg.Namespace},
		},
		HostConfig: container.HostConfig{
			PortBindings:  portBindings,
			RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
			NetworkMode:   DefaultNetworkMode,
		},
	}
	applySourceBuild(&svc, shard.Path)
	return svc
}

// applySourceBuild marks out as built from project Go source via the embedded
// Dockerfile "runtime" target (Go toolchain + distroless base stages). Shared by
// Cardinal shards and built-from-source game services.
func applySourceBuild(out *Service, shardPath string) {
	out.BuildTarget = "runtime"
	out.BuildArgs = map[string]string{"SOURCE_PATH": ".", "SHARD_PATH": shardPath}
	out.Dependencies = []Service{
		{Name: GoBuilderImage, Config: container.Config{Image: GoBuilderImage}},
		{Name: BaseImage, Config: container.Config{Image: BaseImage}},
	}
}

// buildCardinalEnv builds Cardinal's env: the base env plus DB_DSN (the shared
// project database DSN) when the project uses a database.
//
// A shard receives DB_DSN only when the project declares a [[services]] entry with
// db = true (or a config_db service); projectDBDSN returns "" otherwise. So a shard
// using cardinal.data with no such service gets no DB_DSN.
func buildCardinalEnv(cfg *Config, shard worldtoml.Shard) []string {
	env := []string{
		fmt.Sprintf("CARDINAL_REGION=%s", cardinalRegion),
		fmt.Sprintf("CARDINAL_ORG=%s", cfg.WorldToml.Organization),
		fmt.Sprintf("CARDINAL_PROJECT=%s", cfg.WorldToml.Project),
		fmt.Sprintf("CARDINAL_SHARD_ID=%s", shard.InstanceID),
		fmt.Sprintf("NATS_URL=%s", cfg.NATSURL),
		fmt.Sprintf("LOG_LEVEL=%s", shard.LogLevel),
		fmt.Sprintf("CARDINAL_DEBUG=%t", cfg.Debug),
		"LOG_FORMAT=pretty",
		// No OTLP collector in local docker; disable sampling to silence the
		// exporter's "produced zero addresses" spam.
		"OTEL_TRACE_SAMPLE_RATE=0.0",
	}
	// DB_DSN is injected only when the project uses a database (projectDBDSN returns
	// the shared DSN); shards that don't use the data plugin (e.g. lobby) ignore it.
	if dsn := projectDBDSN(cfg.WorldToml); dsn != "" {
		env = append(env, fmt.Sprintf("%s=%s", DBDSNEnvVar, dsn))
	}
	return env
}

// ProjectDBContainerName returns a project's shared database container/host name
// ("{project}-db"). One database per project, reached via DB_DSN.
func ProjectDBContainerName(project string) string {
	return project + "-db"
}

// anyServiceUsesDB reports whether any [[services]] entry sets db = true.
func anyServiceUsesDB(worldToml worldtoml.Config) bool {
	for _, svc := range worldToml.Services {
		if svc.DB {
			return true
		}
	}
	return false
}

// hasConfigDBService reports whether world.toml declares a config_db service
// (the legacy, hand-declared project DB).
func hasConfigDBService(worldToml worldtoml.Config) bool {
	for _, svc := range worldToml.Services {
		if svc.ConfigDB {
			return true
		}
	}
	return false
}

// NeedsAutoProjectDB reports whether the docker backend must auto-provision the
// shared "{project}-db" Postgres: some service set db = true and no config_db
// service was declared to satisfy it.
func NeedsAutoProjectDB(worldToml worldtoml.Config) bool {
	return anyServiceUsesDB(worldToml) && !hasConfigDBService(worldToml)
}

// projectDBDSN returns the shared DB_DSN — one database per project. It prefers a
// declared config_db service (legacy), else the auto-provisioned "{project}-db".
// Returns "" when the project uses no database.
func projectDBDSN(worldToml worldtoml.Config) string {
	if dsn := configDBDSN(worldToml); dsn != "" {
		return dsn
	}
	if anyServiceUsesDB(worldToml) {
		return fmt.Sprintf(
			"postgres://%s:%s@%s:%d/%s?sslmode=disable",
			configDBUser,
			configDBPassword,
			ProjectDBContainerName(worldToml.Project),
			defaultConfigDBPort,
			worldToml.Project,
		)
	}
	return ""
}

// configDBDSN returns the DSN of the first config_db [[services]] entry, or "".
// Credentials/database are world-cli defaults (not read from world.toml); host is
// the service container name and port is the in-network Postgres port (5432).
func configDBDSN(worldToml worldtoml.Config) string {
	for _, svc := range worldToml.Services {
		if !svc.ConfigDB {
			continue
		}
		// The DSN connects in-network to the container's Postgres port (5432), not
		// any host-published port; validateServices pins a declared config_db port to
		// 5432, so the host binding and the DSN stay consistent.
		host := GameServiceContainerName(worldToml.Project, svc.ID)
		return fmt.Sprintf(
			"postgres://%s:%s@%s:%d/%s?sslmode=disable",
			configDBUser, configDBPassword, host, defaultConfigDBPort, worldToml.Project,
		)
	}
	return ""
}
