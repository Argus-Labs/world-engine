package toml

// SourceServiceIDs returns the ID of each [[services]] entry that builds from
// project Go source (Path set), in declaration order. These are the services that
// ride the Cardinal shard build pipeline and receive per-reload registry push
// tags in DeployServices — so they, like shards, need their k3d-registry tags
// cleaned up on `world purge --image`. Image-kind (pulled) entries are excluded:
// they carry no local source image and no registry push tag, so there is nothing
// to remove. IDs are deduplicated defensively (validation already guarantees
// uniqueness) to mirror ShardIDs.
func (c Config) SourceServiceIDs() []string {
	seen := make(map[string]struct{}, len(c.Services))
	out := make([]string, 0, len(c.Services))
	for _, s := range c.Services {
		if !s.IsBuiltFromSource() {
			continue
		}
		if _, ok := seen[s.ID]; ok {
			continue
		}
		seen[s.ID] = struct{}{}
		out = append(out, s.ID)
	}
	return out
}
