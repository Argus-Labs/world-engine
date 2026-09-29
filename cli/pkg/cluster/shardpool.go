package cluster

import (
	"fmt"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/cli/pkg/toml"
)

// DefaultRegion is the region for all local (k3d) shards + [[services]]. It
// matches the docker backend so the same game-client config reaches both.
// Override a single service via [[services.env]] CARDINAL_REGION.
const DefaultRegion = "us-west1"

// ShardPool CR API identity — single source of truth for shardPoolYAML's
// apiVersion/kind and shardPoolGVK. Change together with the CRD.
//
// Mirrors apps/cardinal-operator/api/v1's SchemeGroupVersion + ShardPool kind;
// kept as local constants rather than importing the operator's typed API so
// pkg/cluster (and world-cli) don't pull in controller-runtime.
const (
	shardPoolGroup   = "cardinal.argus.gg"
	shardPoolVersion = "v1"
	shardPoolKind    = "ShardPool"
)

// ShardPool is the plain-Go representation of a ShardPool CR, ready to be
// converted to a Kubernetes object when applied via client-go. The shape
// mirrors apps/cardinal-operator/api/v1.ShardPoolSpec.
type ShardPool struct {
	ShardID      string
	Organization string
	Project      string
	Region       string

	Image    string
	ImageTag string

	TickRate int32
	Mode     string

	PoolSize int32

	Resources Resources

	LogLevel string
}

// Resources is the request + limit pair for a ShardPool.
type Resources struct {
	Requests ResourceValues
	Limits   ResourceValues
}

// ResourceValues uses milliCPU for CPU and MiB for Memory.
type ResourceValues struct {
	CPU    int32
	Memory int32
}

// DefaultResources returns the resource requests/limits applied when a shard
// in world.toml doesn't specify them. Matches the values used by Rampage's
// current dev-cluster ShardPools.
func DefaultResources() Resources {
	return Resources{
		Requests: ResourceValues{CPU: 250, Memory: 256},
		Limits:   ResourceValues{CPU: 1000, Memory: 1024},
	}
}

// ShardPoolsFromConfig translates a world.toml Config into one ShardPool per
// shard ID. Pool expansion (pool_size > 1 in world.toml) collapses
// back into a single ShardPool with PoolSize set to the instance count.
// Backend defaults apply for fields the TOML doesn't specify.
func (c *Client) ShardPoolsFromConfig(cfg toml.Config) ([]ShardPool, error) {
	if cfg.Organization == "" || cfg.Project == "" {
		return nil, eris.New("cluster: world.toml must declare organization and project")
	}

	// Group expanded shards by shard ID so we can compute PoolSize.
	// Preserve TOML declaration order across groups.
	type group struct {
		first     toml.Shard
		instances int32
	}
	groups := make(map[string]*group, len(cfg.Shards))
	order := make([]string, 0, len(cfg.Shards))
	for _, s := range cfg.Shards {
		if g, ok := groups[s.ID]; ok {
			g.instances++
			continue
		}
		groups[s.ID] = &group{first: s, instances: 1}
		order = append(order, s.ID)
	}

	pools := make([]ShardPool, 0, len(order))
	for _, id := range order {
		g := groups[id]
		s := g.first

		// Resources: pass through what the TOML provides, otherwise default.
		resources := DefaultResources()
		if s.Resources != nil {
			resources = Resources{
				Requests: ResourceValues{CPU: s.Resources.Requests.CPU, Memory: s.Resources.Requests.Memory},
				Limits:   ResourceValues{CPU: s.Resources.Limits.CPU, Memory: s.Resources.Limits.Memory},
			}
		}

		pools = append(pools, ShardPool{
			ShardID:      s.ID,
			Organization: cfg.Organization,
			Project:      cfg.Project,
			Region:       DefaultRegion,
			Image:        c.imageRef(cfg.Project, s.ID),
			ImageTag:     "", // runtime-managed by the operator's Deploy RPC
			TickRate:     s.TickRate,
			Mode:         s.Mode,
			PoolSize:     g.instances,
			Resources:    resources,
			LogLevel:     s.LogLevel,
		})
	}

	return pools, nil
}

// imageRef builds the per-shard image reference targeting the local k3d
// registry, e.g. "k3d-world-engine-registry.localhost:5000/rampage/gameplay".
func (c *Client) imageRef(project, shardID string) string {
	return fmt.Sprintf("k3d-%s.localhost:5000/%s/%s", c.cfg.RegistryName, project, shardID)
}

// WorldKey is a world's stable identity, "{org}/{project}". The editor matches it
// against the org/project read back from running ShardPool CRs (DeployedWorldKeys)
// to re-derive run-state after a restart. It's an identity string, not a k8s name
// (shards share one fixed namespace, ShardNamespace), so it needs no DNS-1123
// sanitizing; the slash keeps the two fields unambiguous.
func WorldKey(org, project string) string {
	return org + "/" + project
}
