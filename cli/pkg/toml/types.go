package toml

const FileName = "world.toml"

type Config struct {
	Organization string        `toml:"organization" json:"organization"`
	Project      string        `toml:"project"      json:"project"`
	Auth         Auth          `toml:"auth"         json:"auth"`
	Shards       []Shard       `toml:"shards"       json:"shards"`
	Services     []GameService `toml:"services"     json:"services"`
}

// Shard auth modes for the [auth] section.
const (
	AuthModeDev   = "dev"
	AuthModeArgus = "argus"
)

// Auth is how every shard `world start` runs authenticates players. Deployed shards don't read
// world.toml; their deployment sets CARDINAL_AUTH_MODE and CARDINAL_ARGUS_AUTH_URL instead.
type Auth struct {
	// Mode is AuthModeArgus (validate Argus Auth game tokens) or AuthModeDev (trust the
	// X-Player-Id header). validate sets it to AuthModeDev when world.toml leaves it out.
	Mode string `toml:"mode,omitempty" json:"mode,omitempty"`
	// URL is the Argus Auth service as the shards reach it. Required for AuthModeArgus; ignored
	// for AuthModeDev, so switching modes doesn't mean deleting it.
	URL string `toml:"url,omitempty" json:"url,omitempty"`
}

type Shard struct {
	ID         string `toml:"id"                    json:"id"`
	LogLevel   string `toml:"log_level,omitempty"   json:"logLevel,omitempty"`
	EnableOTEL bool   `toml:"enable_otel,omitempty" json:"enableOtel,omitempty"`
	Path       string `toml:"path,omitempty"        json:"path,omitempty"`
	PoolSize   int    `toml:"pool_size,omitempty"   json:"poolSize,omitempty"`

	// TickRate is the game-loop frequency in Hz. Optional; k8s backend only.
	TickRate int32 `toml:"tick_rate,omitempty" json:"tickRate,omitempty"`

	// Mode is the shard mode (e.g. "LEADER"). Optional; k8s backend only.
	Mode string `toml:"mode,omitempty" json:"mode,omitempty"`

	// Resources is the per-instance CPU + memory request/limit pair.
	// Optional; k8s backend only. Nil means "use translator default."
	Resources *ShardResources `toml:"resources,omitempty" json:"resources,omitempty"`

	// InstanceID is the runtime shard instance ID after pool expansion. Pooled
	// replicas keep ID as the logical world.toml shard ID and get suffixed
	// InstanceIDs such as "game-2". Set during validation, not part of the schema.
	InstanceID string `toml:"-" json:"-"`
}

// ShardResources is the requests + limits pair, mirroring the ShardPool CRD.
type ShardResources struct {
	Requests ShardResourceValues `toml:"requests" json:"requests"`
	Limits   ShardResourceValues `toml:"limits"   json:"limits"`
}

// ShardResourceValues uses milliCPU for CPU and MiB for Memory.
type ShardResourceValues struct {
	CPU    int32 `toml:"cpu"    json:"cpu"`
	Memory int32 `toml:"memory" json:"memory"`
}

// GameService is an auxiliary [[services]] container run alongside the shards by
// the docker backend (e.g. postgres, a headless service). Exactly one of Image
// (pulled) or Path (built from project Go source) is set. Ignored by k8s.
type GameService struct {
	ID          string            `toml:"id"                    json:"id"`
	Image       string            `toml:"image,omitempty"       json:"image,omitempty"`
	Path        string            `toml:"path,omitempty"        json:"path,omitempty"`
	Env         map[string]string `toml:"env,omitempty"         json:"env,omitempty"`
	Ports       []int             `toml:"ports,omitempty"       json:"ports,omitempty"`       // host port == container port
	Volume      string            `toml:"volume,omitempty"      json:"volume,omitempty"`      // container mount path for a named volume
	Healthcheck []string          `toml:"healthcheck,omitempty" json:"healthcheck,omitempty"` // docker healthcheck Test array

	// ConfigDB marks the config database shards read game config from; the docker
	// backend derives DB_DSN from it and injects it into every shard's env.
	ConfigDB bool `toml:"config_db,omitempty" json:"config_db,omitempty"`

	// DB opts this source-built service into the project database (one DB per
	// project, shared by all shards and services). The docker backend
	// auto-provisions a single "{project}-db" Postgres (unless a config_db service
	// already provides one) and injects its DSN as DB_DSN. An explicit env wins.
	DB bool `toml:"db,omitempty" json:"db,omitempty"`
}

// IsBuiltFromSource reports whether the service builds from project Go source
// (Path set) vs. running a pulled image (Image set). validate guarantees exactly
// one is set.
func (g GameService) IsBuiltFromSource() bool { return g.Path != "" }
