package interfaces

// ComponentFilter is a filter that filters entities based on their components.
type IComponentFilter interface {
	// MatchesComponents returns true if the entity matches the filter.
	MatchesComponents(components []IComponentType) bool
}
