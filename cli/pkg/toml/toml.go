package toml

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func LoadFile(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, eris.Wrapf(err, "open %s", path)
	}
	defer func() { _ = f.Close() }()
	return Load(f)
}

func Load(r io.Reader) (Config, error) {
	var cfg Config
	dec := toml.NewDecoder(r)

	// Decode and capture metadata so we could detect unknown/unused keys if desired.
	md, err := dec.Decode(&cfg)
	if err != nil {
		return cfg, eris.Wrap(err, "decode toml")
	}

	if u := md.Undecoded(); len(u) > 0 {
		var keys []string
		for _, k := range u {
			keys = append(keys, k.String())
		}
		log.Warn().Msgf("ignored unknown config keys: %s", strings.Join(keys, ", "))
	}

	if err := validate(&cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

//nolint:gocognit // This function is complex but it's ok for this use case.
func validate(cfg *Config) error {
	if cfg.Organization == "" {
		return eris.New("world.toml is missing an Organization")
	}

	if cfg.Project == "" {
		return eris.New("world.toml is missing an Project")
	}

	// Validate organization
	if err := normalizeAndValidateString(&cfg.Organization, "organization"); err != nil {
		return err
	}

	// Validate project
	if err := normalizeAndValidateCanonicalName(&cfg.Project, "project"); err != nil {
		return err
	}

	if err := validateAuth(&cfg.Auth); err != nil {
		return err
	}

	if len(cfg.Shards) == 0 {
		return eris.New("at least one shard is required")
	}

	seen := make(map[string]bool)
	for i := range cfg.Shards {
		// Raw empty ID means the key is missing entirely
		if cfg.Shards[i].ID == "" {
			return eris.New(fmt.Sprintf("shards[%d].id is required", i))
		}

		// Normalize and validate in-place so changes persist
		if err := normalizeAndValidateCanonicalName(&cfg.Shards[i].ID, "shardID"); err != nil {
			return err
		}

		// Check duplicates after normalization to catch collisions like "shard 1" -> "shard1"
		if seen[cfg.Shards[i].ID] {
			return eris.New(fmt.Sprintf("duplicate shard id %q at index %d", cfg.Shards[i].ID, i))
		}

		// Set default path if not provided
		if cfg.Shards[i].Path == "" {
			cfg.Shards[i].Path = fmt.Sprintf("shards/%s/", cfg.Shards[i].ID)
		}
		seen[cfg.Shards[i].ID] = true

		// Default pool_size: any non-positive value (zero or negative) means
		// "run a single instance". TOML's int zero-value is also 0, so this
		// also covers the "field omitted" case.
		if cfg.Shards[i].PoolSize < 1 {
			cfg.Shards[i].PoolSize = 1
		}

		// Validate shard log level
		if err := validateLogLevel(cfg.Shards[i].LogLevel, "shard: "+cfg.Shards[i].ID); err != nil ||
			cfg.Shards[i].LogLevel == "" {
			cfg.Shards[i].LogLevel = zerolog.InfoLevel.String()
		}

		// Validate shard path
		if err := validatePath(cfg.Shards[i].Path); err != nil || cfg.Shards[i].Path == "" {
			return err
		}

		// Validate tick_rate. Zero is allowed: it's omitted from the ShardPool
		// CR, so the operator skips CARDINAL_TICK_RATE and the shard binary
		// falls back to its own default tick rate.
		if cfg.Shards[i].TickRate < 0 {
			return eris.New(fmt.Sprintf("shards[%d].tick_rate must be non-negative", i))
		}

		// Validate resources if present. The ShardPool CRD marks
		// requests.{cpu,memory} and limits.{cpu,memory} all required with a
		// minimum of 1, and kubelet rejects a limit below its request. The CR
		// encoder emits every value (no omitempty), so a zero/omitted field
		// reaches the API server as 0 and is rejected at apply time. Enforce the
		// full constraint here so the error names the world.toml field instead of
		// surfacing later at operator reconcile.
		if r := cfg.Shards[i].Resources; r != nil {
			if r.Requests.CPU < 1 || r.Requests.Memory < 1 {
				return eris.New(fmt.Sprintf(
					"shards[%d].resources.requests.cpu and .memory must each be >= 1 (got cpu=%d, memory=%d)",
					i, r.Requests.CPU, r.Requests.Memory))
			}
			if r.Limits.CPU < 1 || r.Limits.Memory < 1 {
				return eris.New(fmt.Sprintf(
					"shards[%d].resources.limits.cpu and .memory must each be >= 1 (got cpu=%d, memory=%d)",
					i, r.Limits.CPU, r.Limits.Memory))
			}
			if r.Limits.CPU < r.Requests.CPU {
				return eris.New(fmt.Sprintf(
					"shards[%d].resources.limits.cpu (%d) must be >= requests.cpu (%d)",
					i, r.Limits.CPU, r.Requests.CPU))
			}
			if r.Limits.Memory < r.Requests.Memory {
				return eris.New(fmt.Sprintf(
					"shards[%d].resources.limits.memory (%d) must be >= requests.memory (%d)",
					i, r.Limits.Memory, r.Requests.Memory))
			}
		}
	}

	// Expand pool_size > 1 into multiple shard entries. Naming mirrors
	// cardinal-operator: the first instance keeps the base ID, subsequent
	// instances are numbered from 2 (e.g. "game", "game-2", "game-3").
	expanded, err := expandPools(cfg.Shards)
	if err != nil {
		return err
	}
	cfg.Shards = expanded

	if err := validateServices(cfg); err != nil {
		return err
	}

	return nil
}

// validateServices validates the [[services]] section. Unlike shards there is no
// pool expansion or defaulting.
func validateServices(cfg *Config) error {
	seen := make(map[string]bool)
	configDBSeen := false
	for i := range cfg.Services {
		svc := &cfg.Services[i]
		if svc.ID == "" {
			return eris.New(fmt.Sprintf("services[%d].id is required", i))
		}

		if err := normalizeAndValidateCanonicalName(&svc.ID, "serviceID"); err != nil {
			return err
		}

		if seen[svc.ID] {
			return eris.New(fmt.Sprintf("duplicate service id %q at index %d", svc.ID, i))
		}
		seen[svc.ID] = true

		// Exactly one of image (pulled) or path (built from source) must be set.
		if (svc.Image == "") == (svc.Path == "") {
			return eris.New(fmt.Sprintf("services[%d] (%s) must set exactly one of image or path", i, svc.ID))
		}

		// config_db (the shared project DB) and db (a consumer of that DB) are
		// mutually exclusive: a single service cannot both provide and consume it.
		if svc.ConfigDB && svc.DB {
			return eris.New(fmt.Sprintf("services[%d] (%s) cannot set both config_db and db", i, svc.ID))
		}

		// One database per project: at most one config_db service (the shared
		// project DB), and it must expose a single port (the Postgres port) so its
		// derived DSN is unambiguous. A second config_db, or an ambiguous multi-port
		// one, is a misconfiguration.
		if svc.ConfigDB {
			if configDBSeen {
				return eris.New(
					fmt.Sprintf(
						"services[%d] (%s) declares config_db but another config_db service already exists; only one database per project is allowed",
						i,
						svc.ID,
					),
				)
			}
			configDBSeen = true
			if len(svc.Ports) > 1 {
				return eris.New(
					fmt.Sprintf(
						"services[%d] (%s) config_db must declare at most one port (the Postgres port), got %d",
						i,
						svc.ID,
						len(svc.Ports),
					),
				)
			}
			// config_db is a stock Postgres listening on 5432 in-network; a declared
			// port publishes that same port to the host, so it must be 5432 (or be
			// omitted) or the DSN and the host binding would diverge.
			if len(svc.Ports) == 1 && svc.Ports[0] != 5432 {
				return eris.New(
					fmt.Sprintf(
						"services[%d] (%s) config_db port must be 5432 (the Postgres port), got %d",
						i,
						svc.ID,
						svc.Ports[0],
					),
				)
			}
			// The config_db is an auto-provisioned, pulled Postgres image; it
			// cannot be built from source. Reject path so the misconfiguration
			// surfaces here rather than as a confusing build attempt.
			if svc.Path != "" {
				return eris.New(fmt.Sprintf("services[%d] (%s) config_db must use image, not path", i, svc.ID))
			}
		}

		// Docker requires the healthcheck Test array's first element to be CMD,
		// CMD-SHELL, or NONE; reject others here rather than failing cryptically at
		// container creation.
		if len(svc.Healthcheck) > 0 {
			switch svc.Healthcheck[0] {
			case "CMD", "CMD-SHELL", "NONE":
			default:
				return eris.New(
					fmt.Sprintf(
						"services[%d] (%s) healthcheck[0] must be CMD, CMD-SHELL, or NONE, got %q",
						i,
						svc.ID,
						svc.Healthcheck[0],
					),
				)
			}
		}

		if svc.Path != "" {
			if err := validatePath(svc.Path); err != nil {
				return err
			}
		}

		for _, port := range svc.Ports {
			if port < 1 || port > 65535 {
				return eris.New(fmt.Sprintf("services[%d] (%s) port %d must be between 1 and 65535", i, svc.ID, port))
			}
		}

		if svc.Volume != "" && !strings.HasPrefix(svc.Volume, "/") {
			return eris.New(fmt.Sprintf("services[%d] (%s) volume must be an absolute container path", i, svc.ID))
		}
	}
	return nil
}

func expandPools(shards []Shard) ([]Shard, error) {
	total := 0
	for _, s := range shards {
		total += s.PoolSize
	}
	expanded := make([]Shard, 0, total)
	// final instance ID -> shard ID that produced it. Lets us name both
	// sides when a pool_size expansion collides with another shard's instance ID.
	source := make(map[string]string, total)
	for _, s := range shards {
		baseID := s.ID
		for r := 1; r <= s.PoolSize; r++ {
			entry := s
			instanceID := baseID
			if r > 1 {
				instanceID = fmt.Sprintf("%s-%d", baseID, r)
			}
			entry.PoolSize = 1
			entry.InstanceID = instanceID
			if prev, ok := source[entry.InstanceID]; ok {
				return nil, eris.New(fmt.Sprintf(
					"shard instance id %q is produced by both %q and %q; rename a shard or reduce pool_size",
					entry.InstanceID, prev, baseID,
				))
			}
			source[entry.InstanceID] = baseID
			expanded = append(expanded, entry)
		}
	}
	return expanded, nil
}
