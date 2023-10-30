package filter

import component_metadata "pkg.world.dev/world-engine/cardinal/ecs/component/metadata"

// ComponentFilter is a filter that filters entities based on their components.
type ComponentFilter interface {
	// MatchesComponents returns true if the entity matches the filter.
	MatchesComponents(components []component_metadata.ComponentMetadata) bool
}
