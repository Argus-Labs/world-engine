package service

import "strings"

// VolumeNames returns the deterministic named volumes world creates for the
// project's platform and [[services]] containers, so purge can remove them by
// name without depending on their containers still existing — a volume whose
// container was already removed (a failed recreate, a manual `docker rm`, or
// `docker container prune`) is invisible to the mount walk inside
// RemoveContainers, so without this list purge would leave it on disk and the
// next `world start` would bind the same name and resurrect the old state.
//
// Shard containers carry no volumes, so they are intentionally not built here.
// The names mirror the bind declarations in NATS, ProjectDBService and
// GameServiceFromConfig: the volume name is the source of each "<name>:<path>"
// bind, so a volume added to a service definition is picked up here
// automatically rather than naming logic being duplicated.
func VolumeNames(cfg *Config) []string {
	svcs := []Service{NATS(cfg)}
	if NeedsAutoProjectDB(cfg.WorldToml) {
		svcs = append(svcs, ProjectDBService(cfg))
	}
	for _, gs := range cfg.WorldToml.Services {
		svcs = append(svcs, GameServiceFromConfig(cfg, gs))
	}
	var names []string
	for _, s := range svcs {
		for _, b := range s.Binds {
			vol, _, ok := strings.Cut(b, ":")
			if !ok || vol == "" {
				continue
			}
			names = append(names, vol)
		}
	}
	return names
}
