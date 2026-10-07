package service

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/moby/moby/api/types/container"

	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

const (
	gameServiceHealthInterval = 2 * time.Second
	gameServiceHealthTimeout  = 3 * time.Second
	gameServiceHealthRetries  = 15
)

// GameServiceContainerName returns the container name for a [[services]] entry.
// Keyed off the world.toml project (not the directory-derived namespace) so
// hostname-referencing env values (e.g. a postgres DSN) stay deterministic across
// checkout directories.
func GameServiceContainerName(project, id string) string {
	return fmt.Sprintf("%s-%s-service", project, id)
}

// BuildGameServices returns one builder per [[services]] entry in declaration
// order (which drives start order). When a service sets db = true and no config_db
// service satisfies it, the auto-provisioned "{project}-db" Postgres is prepended
// so it goes healthy first; it rides here (not just the start path) so stop/purge
// tear it down with the project.
func BuildGameServices(cfg *Config) []Builder {
	services := make([]Builder, 0, len(cfg.WorldToml.Services)+1)
	if NeedsAutoProjectDB(cfg.WorldToml) {
		services = append(services, ProjectDBService)
	}
	for _, sc := range cfg.WorldToml.Services {
		svcCfg := sc
		services = append(services, func(c *Config) Service {
			return GameServiceFromConfig(c, svcCfg)
		})
	}
	return services
}

// GameServiceFromConfig builds the docker service for one [[services]] entry.
// Image-kind entries run a pulled image; path-kind entries ride the Cardinal shard
// build pipeline, which selects them by their build target (see IsCardinalService).
func GameServiceFromConfig(cfg *Config, svc worldtoml.GameService) Service {
	out := buildBaseGameService(cfg, svc)
	if svc.IsBuiltFromSource() {
		out.Image = out.Name
		applySourceBuild(&out, svc.Path)
	} else {
		out.Image = svc.Image
	}
	return out
}

// buildBaseGameService builds the parts shared by pulled and built-from-source
// [[services]] entries (name, env, labels, ports, volume, healthcheck, restart,
// network). The caller sets the image and build pipeline per service kind.
func buildBaseGameService(cfg *Config, svc worldtoml.GameService) Service {
	containerName := GameServiceContainerName(cfg.WorldToml.Project, svc.ID)

	out := Service{
		Name:        containerName,
		Env:         buildGameServiceEnv(cfg, svc),
		Labels:      Labels(cfg.WorldToml.Project, RoleService, "", svc.ID),
		NetworkMode: container.NetworkMode(NetworkName(cfg.WorldToml.Project)),
	}

	if len(svc.Ports) > 0 {
		out.ExposedPorts = getExposedPorts(svc.Ports)
		out.PortBindings = newPortMap(svc.Ports)
	}
	if svc.Volume != "" {
		// Volume name == container name so purge's implicit volume delete cleans it up.
		out.Binds = []string{fmt.Sprintf("%s:%s", containerName, svc.Volume)}
	}
	if len(svc.Healthcheck) > 0 {
		out.Healthcheck = &container.HealthConfig{
			Test:     svc.Healthcheck,
			Interval: gameServiceHealthInterval,
			Timeout:  gameServiceHealthTimeout,
			Retries:  gameServiceHealthRetries,
		}
	}

	return out
}

// buildGameServiceEnv merges the env sources into a single deduplicated
// "KEY=VALUE" slice sorted by key (deterministic). Precedence is per-key, not
// uniformly "world-cli wins":
//
//   - NATS_URL / CARDINAL_REGION: world.toml wins. They're seeded as world-cli
//     defaults first, then svc.Env is applied on top, overwriting any matching key.
//   - DB_DSN (db = true): world.toml wins. world-cli only fills the shared DSN
//     when svc.Env didn't already set DB_DSN.
//   - POSTGRES_USER / POSTGRES_PASSWORD / POSTGRES_DB (config_db): world-cli's
//     config_db credentials win. They're written after svc.Env, overriding any
//     world.toml values, so they stay consistent with the DSN world-cli builds.
func buildGameServiceEnv(cfg *Config, svc worldtoml.GameService) []string {
	// NATS_URL/CARDINAL_REGION seed every service, including a config_db Postgres
	// (which ignores them); kept uniform to avoid a per-kind env branch.
	merged := map[string]string{
		"NATS_URL":        cfg.NATSURL,
		"CARDINAL_REGION": CardinalRegion,
	}
	// svc.Env (world.toml) overwrites the world-cli seeds above, so world.toml
	// wins for NATS_URL/CARDINAL_REGION.
	maps.Copy(merged, svc.Env)
	if svc.DB {
		if dsn := projectDBDSN(cfg.WorldToml); dsn != "" {
			// A world.toml DB_DSN wins, so only fill it in when absent.
			if _, ok := merged[DBDSNEnvVar]; !ok {
				merged[DBDSNEnvVar] = dsn
			}
		}
	}
	if svc.ConfigDB {
		merged["POSTGRES_USER"] = configDBUser
		merged["POSTGRES_PASSWORD"] = configDBPassword
		merged["POSTGRES_DB"] = cfg.WorldToml.Project
	}

	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	env := make([]string, 0, len(merged))
	for _, k := range keys {
		env = append(env, fmt.Sprintf("%s=%s", k, merged[k]))
	}
	return env
}
