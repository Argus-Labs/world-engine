package toml

import (
	"slices"
	"strconv"
	"strings"

	"github.com/rotisserie/eris"
)

// ShardIDs returns each world.toml shard ID once, in declaration order.
func (c Config) ShardIDs() []string {
	seen := make(map[string]struct{}, len(c.Shards))
	out := make([]string, 0, len(c.Shards))
	for _, s := range c.Shards {
		if _, ok := seen[s.ID]; ok {
			continue
		}
		seen[s.ID] = struct{}{}
		out = append(out, s.ID)
	}
	return out
}

// ResolveInstanceIDs returns the shard instances named by ids. Empty input
// selects every instance. Blank and duplicate IDs are ignored, request order is
// preserved, and any unknown ID fails the whole selection.
func (c Config) ResolveInstanceIDs(ids []string) ([]Shard, error) {
	if len(ids) == 0 {
		return slices.Clone(c.Shards), nil
	}

	instancesByID := make(map[string]Shard, len(c.Shards))
	for _, instance := range c.Shards {
		instancesByID[instance.InstanceID] = instance
	}

	seen := make(map[string]struct{}, len(ids))
	out := make([]Shard, 0, len(ids))
	var unknown []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if _, duplicate := seen[id]; id == "" || duplicate {
			continue
		}
		seen[id] = struct{}{}

		instance, ok := instancesByID[id]
		if !ok {
			unknown = append(unknown, strconv.Quote(id))
			continue
		}
		out = append(out, instance)
	}
	if len(seen) == 0 {
		return slices.Clone(c.Shards), nil
	}
	if len(unknown) != 0 {
		return nil, eris.Errorf("instance IDs not found in %s: %s", FileName, strings.Join(unknown, ", "))
	}
	return out, nil
}
