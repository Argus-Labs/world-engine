package service

// VolumeNames lists the named volumes world creates for a project: the NATS
// store, the auto project DB and one per [[services]] entry with a volume.
// Purge removes them by name so a volume whose container is already gone is
// still reclaimed. The list comes from the current world.toml, so a volume of
// an entry removed from the config is not covered. The DB volume is always
// included: it may predate a config_db service, and not-found is skipped.
func VolumeNames(cfg *Config) []string {
	project := cfg.WorldToml.Project
	names := []string{NatsVolumeName(project), ProjectDBVolumeName(project)}
	for _, gs := range cfg.WorldToml.Services {
		if gs.Volume != "" {
			names = append(names, GameServiceContainerName(project, gs.ID))
		}
	}
	return names
}
