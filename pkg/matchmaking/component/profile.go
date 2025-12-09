package component

// StringEqualsFilter matches when a field exactly equals a value.
type StringEqualsFilter struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

// DoubleRangeFilter matches when a field is within a range [min, max].
type DoubleRangeFilter struct {
	Field string  `json:"field"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
}

// TagPresentFilter matches when a tag is present in the tags list.
type TagPresentFilter struct {
	Tag string `json:"tag"`
}

// PoolConfig defines a pool within a match profile.
type PoolConfig struct {
	Name                string               `json:"name"`
	StringEqualsFilters []StringEqualsFilter `json:"string_equals_filters,omitempty"`
	DoubleRangeFilters  []DoubleRangeFilter  `json:"double_range_filters,omitempty"`
	TagPresentFilters   []TagPresentFilter   `json:"tag_present_filters,omitempty"`
	MinPlayers          int                  `json:"min_players"`
	MaxPlayers          int                  `json:"max_players"`
}

// TeamConfig defines a team within a match profile.
type TeamConfig struct {
	Name       string   `json:"name"`
	Pools      []string `json:"pools"`
	MinPlayers int      `json:"min_players"`
	MaxPlayers int      `json:"max_players"`
}

// ProfileComponent represents a match profile configuration.
type ProfileComponent struct {
	ProfileName string       `json:"profile_name"`
	Pools       []PoolConfig `json:"pools"`
	Teams       []TeamConfig `json:"teams"`
	MinPlayers  int          `json:"min_players"`
	MaxPlayers  int          `json:"max_players"`
}

// Name returns the component name for ECS registration.
func (ProfileComponent) Name() string { return "matchmaking_profile" }

// GetProfileName returns the profile name.
func (p *ProfileComponent) GetProfileName() string { return p.ProfileName }

// GetPool returns a pool by name.
func (p *ProfileComponent) GetPool(name string) (PoolConfig, bool) {
	for _, pool := range p.Pools {
		if pool.Name == name {
			return pool, true
		}
	}
	return PoolConfig{}, false
}

// GetTeam returns a team by name.
func (p *ProfileComponent) GetTeam(name string) (TeamConfig, bool) {
	for _, team := range p.Teams {
		if team.Name == name {
			return team, true
		}
	}
	return TeamConfig{}, false
}
