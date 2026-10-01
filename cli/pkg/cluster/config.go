package cluster

// Config holds Client construction inputs. Zero-valued fields fall back to
// Defaults().
type Config struct {
	ClusterName  string // passed to `k3d cluster create`
	RegistryName string // attached registry for shard image refs
	// K3sImage must be explicit when calling k3d as a library — the CLI sets
	// this via -ldflags at link time, so an empty value here yields a
	// rootfs-less server container.
	K3sImage         string
	APIEndpoint      string // localhost URL for Traefik's Cardinal API edge (k3d NodePort)
	OperatorEndpoint string // URL for the operator RPC; localhost NodePort for k3d
	// TokenSource authenticates a remote operator. Nil for k3d, which is unauthenticated.
	TokenSource TokenSource
	DBEndpoint  string // localhost host:port for the project DB (k3d NodePort)
	// LogLevel is a logrus-style string ("trace"/"debug"/"info"/"warn"/
	// "error"); empty defaults to "info". Routes into k3d's internal logger.
	LogLevel string
	// OnK3DLog, if set, receives each essential k3d log line (see
	// k3dEssentialFormatter) instead of it going to os.Stderr — needed by
	// callers with their own terminal renderer, so a second writer doesn't
	// race its redraws. nil preserves the original os.Stderr behavior.
	OnK3DLog func(line string)
}

// Defaults returns a Config populated with standard local-dev defaults.
func Defaults() Config {
	return Config{
		ClusterName:      "world-engine",
		RegistryName:     "world-engine-registry",
		K3sImage:         "docker.io/rancher/k3s:v1.31.5-k3s1",
		APIEndpoint:      "http://localhost:8080",
		OperatorEndpoint: "http://localhost:8090",
		DBEndpoint:       "localhost:5432",
		// Noise reduction is handled by k3dEssentialFormatter (see k3d.go),
		// not by the log level — we keep INFO entries flowing so the filter
		// can promote useful ones (image pulls). Set WORLD_K3D_LOG_LEVEL=debug
		// to see DEBUG entries too (still filtered to essential signals).
		LogLevel: "info",
	}
}

// withDefaults returns cfg with any zero-valued fields replaced by Defaults().
func (cfg Config) withDefaults() Config {
	d := Defaults()
	if cfg.ClusterName == "" {
		cfg.ClusterName = d.ClusterName
	}
	if cfg.RegistryName == "" {
		cfg.RegistryName = d.RegistryName
	}
	if cfg.K3sImage == "" {
		cfg.K3sImage = d.K3sImage
	}
	if cfg.APIEndpoint == "" {
		cfg.APIEndpoint = d.APIEndpoint
	}
	if cfg.OperatorEndpoint == "" {
		cfg.OperatorEndpoint = d.OperatorEndpoint
	}
	if cfg.DBEndpoint == "" {
		cfg.DBEndpoint = d.DBEndpoint
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = d.LogLevel
	}
	return cfg
}
