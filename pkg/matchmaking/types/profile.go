package types

import (
	matchmakingv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/matchmaking/v1"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
)

// Profile is the Go representation of a match profile configuration.
// It mirrors the protobuf definition but uses Go-native types for easier manipulation.
type Profile struct {
	Name            string
	Pools           []Pool
	TeamCount       int
	TeamSize        int
	TeamMinSize     int
	TeamComposition []PoolRequirement
	Teams           []TeamDefinition
	Config          map[string]any
	LobbyAddress    *microv1.ServiceAddress // Lobby Shard address (where to send Match)
	TargetAddress   *microv1.ServiceAddress // Game Shard address (where Lobby sends game-start)
}

// Pool defines filtering criteria to categorize tickets.
type Pool struct {
	Name                string
	StringEqualsFilters []StringEqualsFilter
	DoubleRangeFilters  []DoubleRangeFilter
	TagPresentFilters   []TagPresentFilter
}

// StringEqualsFilter matches when a field exactly equals a value.
type StringEqualsFilter struct {
	Field string
	Value string
}

// DoubleRangeFilter matches when a field is within a range [min, max].
type DoubleRangeFilter struct {
	Field string
	Min   float64
	Max   float64
}

// TagPresentFilter matches when a tag is present in the tags list.
type TagPresentFilter struct {
	Tag string
}

// PoolRequirement specifies how many players from a pool are needed per team.
type PoolRequirement struct {
	Pool  string
	Count int
}

// TeamDefinition defines a team for asymmetric games.
type TeamDefinition struct {
	Name        string
	Size        int
	Composition []PoolRequirement
}

// IsSymmetric returns true if this is a symmetric game (all teams identical).
func (p *Profile) IsSymmetric() bool {
	return len(p.Teams) == 0
}

// GetTeamCount returns the number of teams in the match.
func (p *Profile) GetTeamCount() int {
	if p.IsSymmetric() {
		return p.TeamCount
	}
	return len(p.Teams)
}

// GetTeamSize returns the maximum team size for a given team index.
func (p *Profile) GetTeamSize(teamIndex int) int {
	if p.IsSymmetric() {
		return p.TeamSize
	}
	if teamIndex < len(p.Teams) {
		return p.Teams[teamIndex].Size
	}
	return 0
}

// GetTeamMinSize returns the minimum team size for a given team index.
func (p *Profile) GetTeamMinSize(teamIndex int) int {
	if p.IsSymmetric() {
		if p.TeamMinSize > 0 {
			return p.TeamMinSize
		}
		return p.TeamSize
	}
	if teamIndex < len(p.Teams) {
		return p.Teams[teamIndex].Size // Asymmetric teams have fixed size
	}
	return 0
}

// GetTeamName returns the name for a given team index.
func (p *Profile) GetTeamName(teamIndex int) string {
	if p.IsSymmetric() {
		return teamName(teamIndex)
	}
	if teamIndex < len(p.Teams) {
		return p.Teams[teamIndex].Name
	}
	return ""
}

// GetTeamComposition returns the composition requirements for a team.
func (p *Profile) GetTeamComposition(teamIndex int) []PoolRequirement {
	if p.IsSymmetric() {
		return p.TeamComposition
	}
	if teamIndex < len(p.Teams) {
		return p.Teams[teamIndex].Composition
	}
	return nil
}

// HasRoles returns true if the profile has role requirements.
func (p *Profile) HasRoles() bool {
	if p.IsSymmetric() {
		return len(p.TeamComposition) > 0
	}
	for _, team := range p.Teams {
		if len(team.Composition) > 0 {
			return true
		}
	}
	return false
}

// GetTeamCompositionMap returns the composition requirements for a team as a map.
// Returns map[poolName]count for the given team index.
func (p *Profile) GetTeamCompositionMap(teamIndex int) map[string]int {
	composition := p.GetTeamComposition(teamIndex)
	result := make(map[string]int, len(composition))
	for _, req := range composition {
		result[req.Pool] = req.Count
	}
	return result
}

// TotalPlayersNeeded returns the total number of players needed for a complete match.
func (p *Profile) TotalPlayersNeeded() int {
	if p.IsSymmetric() {
		return p.TeamCount * p.TeamSize
	}
	total := 0
	for _, team := range p.Teams {
		total += team.Size
	}
	return total
}

// teamName generates a team name for symmetric games.
func teamName(index int) string {
	return "team_" + string(rune('1'+index))
}

// ToProto converts a Profile to its protobuf representation.
func (p *Profile) ToProto() *matchmakingv1.MatchProfile {
	proto := &matchmakingv1.MatchProfile{
		Name:          p.Name,
		TeamCount:     int32(p.TeamCount),
		TeamSize:      int32(p.TeamSize),
		TeamMinSize:   int32(p.TeamMinSize),
		LobbyAddress:  p.LobbyAddress,
		TargetAddress: p.TargetAddress,
	}

	for _, pool := range p.Pools {
		pl := &matchmakingv1.Pool{Name: pool.Name}
		for _, f := range pool.StringEqualsFilters {
			pl.StringEqualsFilters = append(pl.StringEqualsFilters, &matchmakingv1.StringEqualsFilter{
				Field: f.Field,
				Value: f.Value,
			})
		}
		for _, f := range pool.DoubleRangeFilters {
			pl.DoubleRangeFilters = append(pl.DoubleRangeFilters, &matchmakingv1.DoubleRangeFilter{
				Field: f.Field,
				Min:   f.Min,
				Max:   f.Max,
			})
		}
		for _, f := range pool.TagPresentFilters {
			pl.TagPresentFilters = append(pl.TagPresentFilters, &matchmakingv1.TagPresentFilter{
				Tag: f.Tag,
			})
		}
		proto.Pools = append(proto.Pools, pl)
	}

	for _, c := range p.TeamComposition {
		proto.TeamComposition = append(proto.TeamComposition, &matchmakingv1.PoolRequirement{
			Pool:  c.Pool,
			Count: int32(c.Count),
		})
	}

	for _, t := range p.Teams {
		team := &matchmakingv1.TeamDefinition{
			Name: t.Name,
			Size: int32(t.Size),
		}
		for _, c := range t.Composition {
			team.Composition = append(team.Composition, &matchmakingv1.PoolRequirement{
				Pool:  c.Pool,
				Count: int32(c.Count),
			})
		}
		proto.Teams = append(proto.Teams, team)
	}

	return proto
}
