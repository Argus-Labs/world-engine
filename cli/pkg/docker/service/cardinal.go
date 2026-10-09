package service

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"

	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

const (
	// CardinalPort is the ConnectRPC port inside every shard container.
	CardinalPort = 8080
	// ShardHostPortBase is the host port of the first shard instance; instance i gets base+i on 127.0.0.1.
	ShardHostPortBase = 8081

	// DBDSNEnvVar carries the project Postgres DSN into shards and services.
	DBDSNEnvVar = "DB_DSN"

	projectDBImage = "postgres:16"

	defaultConfigDBPort = 5432

	configDBUser     = "postgres"
	configDBPassword = "postgres"

	// Local-only env the cardinal-shard chart does not render.
	localLogFormat = "LOG_FORMAT=pretty"

	defaultLogLevel = "info"
	// defaultMode mirrors the chart's values.yaml default; the chart always renders CARDINAL_MODE.
	defaultMode = "LEADER"
	// snapshotStorageType matches what the chart renders locally so reload --purge wipes real state.
	snapshotStorageType = "JETSTREAM"
)

// BuildCardinalShards returns one builder per pool-expanded instance, host port ShardHostPortBase+i.
func BuildCardinalShards(cfg *Config) []Builder {
	services := make([]Builder, 0, len(cfg.WorldToml.Shards))
	for i, sc := range cfg.WorldToml.Shards {
		shardCfg := sc
		hostPort := ShardHostPort(i)
		services = append(services, func(c *Config) Service {
			return CardinalFromShard(c, shardCfg, hostPort)
		})
	}
	return services
}

// ShardHostPort is the 127.0.0.1 port of the i-th instance in world.toml order.
func ShardHostPort(i int) int { return ShardHostPortBase + i }

// CardinalShardContainerName is "<project>-<instance>-shard".
func CardinalShardContainerName(project, instanceID string) string {
	return fmt.Sprintf("%s-%s-shard", project, instanceID)
}

// CardinalShardImageName is "<project>-<shard>-shard"; one image per pool.
func CardinalShardImageName(project, shardID string) string {
	return fmt.Sprintf("%s-%s-shard", project, shardID)
}

// CardinalFromShard builds the container for one shard instance. RestartPolicy is
// "no" so a crash stays visible in docker ps and world logs.
func CardinalFromShard(cfg *Config, shard worldtoml.Shard, hostPort int) Service {
	project := cfg.WorldToml.Project
	tcp := network.MustParsePort(strconv.Itoa(CardinalPort) + "/tcp")
	labels := Labels(project, RoleShard, shard.ID, shard.InstanceID)
	labels[OrgLabel] = cfg.WorldToml.Organization
	svc := Service{
		Name:         CardinalShardContainerName(project, shard.InstanceID),
		Image:        CardinalShardImageName(project, shard.ID),
		Env:          buildCardinalEnv(cfg, shard),
		ExposedPorts: getExposedPorts([]int{CardinalPort}),
		Labels:       labels,
		PortBindings: network.PortMap{tcp: loopbackBinding(hostPort)},
		NetworkMode:  container.NetworkMode(NetworkName(project)),
	}
	applySourceBuild(&svc, shard.Path)
	return svc
}

func applySourceBuild(out *Service, shardPath string) {
	out.BuildTarget = "runtime"
	out.BuildArgs = map[string]string{"SOURCE_PATH": ".", "SHARD_PATH": shardPath}
	out.Dependencies = []Service{
		{Name: GoBuilderImage, Image: GoBuilderImage},
		{Name: BaseImage, Image: BaseImage},
	}
}

// buildCardinalEnv is the local side of the chart contract: the same keys and values
// the cardinal-shard chart renders for a local world, plus localLogFormat.
// The chart lives in monorepo (infra/k8s/apps/cardinal-shard); keep the two in step.
func buildCardinalEnv(cfg *Config, shard worldtoml.Shard) []string {
	logLevel := shard.LogLevel
	if logLevel == "" {
		logLevel = defaultLogLevel
	}
	env := []string{
		"CARDINAL_SHARD_ID=" + shard.InstanceID,
		"CARDINAL_ORG=" + cfg.WorldToml.Organization,
		"CARDINAL_PROJECT=" + cfg.WorldToml.Project,
		"CARDINAL_REGION=" + CardinalRegion,
		"LOG_LEVEL=" + logLevel,
		"CARDINAL_SNAPSHOT_STORAGE_TYPE=" + snapshotStorageType,
		"CARDINAL_AUTH_MODE=" + shardAuthModeEnv(cfg.WorldToml),
		"NATS_URL=" + cfg.NATSURL,
		DBDSNEnvVar + "=" + ShardDBDSN(cfg.WorldToml),
	}
	// An argus world validates real Argus Auth tokens, so it can be played with the
	// same accounts as a hosted one. The chart renders the same pair.
	if shardAuthMode(cfg.WorldToml) == worldtoml.AuthModeArgus {
		env = append(env, "CARDINAL_ARGUS_AUTH_URL="+cfg.WorldToml.Auth.URL)
	}
	if shard.TickRate > 0 {
		env = append(env, fmt.Sprintf("CARDINAL_TICK_RATE=%d", shard.TickRate))
	}
	mode := shard.Mode
	if mode == "" {
		mode = defaultMode
	}
	env = append(env,
		"CARDINAL_MODE="+mode,
		fmt.Sprintf("CARDINAL_DEBUG=%t", cfg.Debug),
		"OTEL_EXPORTER_OTLP_ENDPOINT=",
		fmt.Sprintf("OTEL_RESOURCE_ATTRIBUTES=shard.id=%s,service.instance.id=%s", shard.ID, shard.InstanceID),
		localLogFormat,
	)
	return env
}

// shardAuthMode is the world's auth mode. world.toml validation defaults it to dev;
// callers that build a Config without parsing one (MCP reads by project name) get the
// same default rather than an empty value the shard would reject.
func shardAuthMode(worldToml worldtoml.Config) string {
	if worldToml.Auth.Mode == "" {
		return worldtoml.AuthModeDev
	}
	return worldToml.Auth.Mode
}

// shardAuthModeEnv is the CARDINAL_AUTH_MODE value: cardinal and the chart's schema
// spell the modes in upper case, world.toml in lower.
func shardAuthModeEnv(worldToml worldtoml.Config) string {
	return strings.ToUpper(shardAuthMode(worldToml))
}

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

// NeedsAutoProjectDB reports whether world start runs the shared "<project>-db"
// Postgres. It does for every project, matching the chart (which always renders
// DB_DSN), unless a config_db [[service]] brings its own database.
func NeedsAutoProjectDB(worldToml worldtoml.Config) bool {
	return !hasConfigDBService(worldToml)
}

// ShardDBDSN is every shard's DB_DSN: the config_db service when declared, else "<project>-db". Never "".
func ShardDBDSN(worldToml worldtoml.Config) string {
	if dsn := configDBDSN(worldToml); dsn != "" {
		return dsn
	}
	return autoProjectDBDSN(worldToml.Project)
}

func autoProjectDBDSN(project string) string {
	addr := net.JoinHostPort(ProjectDBContainerName(project), strconv.Itoa(defaultConfigDBPort))
	return fmt.Sprintf(
		"postgres://%s:%s@%s/%s?sslmode=disable",
		configDBUser, configDBPassword, addr, project,
	)
}

// projectDBDSN returns the shared DB_DSN — one database per project. It prefers a
// declared config_db service (legacy), else the auto-provisioned "{project}-db".
// Returns "" when the project uses no database.
func projectDBDSN(worldToml worldtoml.Config) string {
	if dsn := configDBDSN(worldToml); dsn != "" {
		return dsn
	}
	if anyServiceUsesDB(worldToml) {
		return autoProjectDBDSN(worldToml.Project)
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
