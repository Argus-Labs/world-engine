package matchmaking

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/matchmaking/types"
)

func TestPassesStringEqualsFilter(t *testing.T) {
	tests := []struct {
		name     string
		fields   types.SearchFields
		filter   types.StringEqualsFilter
		expected bool
	}{
		{
			name: "exact match",
			fields: types.SearchFields{
				StringArgs: map[string]string{"region": "NA"},
			},
			filter:   types.StringEqualsFilter{Field: "region", Value: "NA"},
			expected: true,
		},
		{
			name: "value mismatch",
			fields: types.SearchFields{
				StringArgs: map[string]string{"region": "EU"},
			},
			filter:   types.StringEqualsFilter{Field: "region", Value: "NA"},
			expected: false,
		},
		{
			name: "field not present",
			fields: types.SearchFields{
				StringArgs: map[string]string{"other": "value"},
			},
			filter:   types.StringEqualsFilter{Field: "region", Value: "NA"},
			expected: false,
		},
		{
			name:     "nil StringArgs",
			fields:   types.SearchFields{},
			filter:   types.StringEqualsFilter{Field: "region", Value: "NA"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := passesStringEqualsFilter(tt.fields, tt.filter)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPassesDoubleRangeFilter(t *testing.T) {
	tests := []struct {
		name     string
		fields   types.SearchFields
		filter   types.DoubleRangeFilter
		expected bool
	}{
		{
			name: "value within range",
			fields: types.SearchFields{
				DoubleArgs: map[string]float64{"elo": 1500},
			},
			filter:   types.DoubleRangeFilter{Field: "elo", Min: 1000, Max: 2000},
			expected: true,
		},
		{
			name: "value at min boundary",
			fields: types.SearchFields{
				DoubleArgs: map[string]float64{"elo": 1000},
			},
			filter:   types.DoubleRangeFilter{Field: "elo", Min: 1000, Max: 2000},
			expected: true,
		},
		{
			name: "value at max boundary",
			fields: types.SearchFields{
				DoubleArgs: map[string]float64{"elo": 2000},
			},
			filter:   types.DoubleRangeFilter{Field: "elo", Min: 1000, Max: 2000},
			expected: true,
		},
		{
			name: "value below min",
			fields: types.SearchFields{
				DoubleArgs: map[string]float64{"elo": 500},
			},
			filter:   types.DoubleRangeFilter{Field: "elo", Min: 1000, Max: 2000},
			expected: false,
		},
		{
			name: "value above max",
			fields: types.SearchFields{
				DoubleArgs: map[string]float64{"elo": 2500},
			},
			filter:   types.DoubleRangeFilter{Field: "elo", Min: 1000, Max: 2000},
			expected: false,
		},
		{
			name: "field not present",
			fields: types.SearchFields{
				DoubleArgs: map[string]float64{"other": 1500},
			},
			filter:   types.DoubleRangeFilter{Field: "elo", Min: 1000, Max: 2000},
			expected: false,
		},
		{
			name:     "nil DoubleArgs",
			fields:   types.SearchFields{},
			filter:   types.DoubleRangeFilter{Field: "elo", Min: 1000, Max: 2000},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := passesDoubleRangeFilter(tt.fields, tt.filter)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPassesTagPresentFilter(t *testing.T) {
	tests := []struct {
		name     string
		fields   types.SearchFields
		filter   types.TagPresentFilter
		expected bool
	}{
		{
			name: "tag present",
			fields: types.SearchFields{
				Tags: []string{"ranked_ready", "verified"},
			},
			filter:   types.TagPresentFilter{Tag: "ranked_ready"},
			expected: true,
		},
		{
			name: "tag not present",
			fields: types.SearchFields{
				Tags: []string{"verified"},
			},
			filter:   types.TagPresentFilter{Tag: "ranked_ready"},
			expected: false,
		},
		{
			name: "empty tags",
			fields: types.SearchFields{
				Tags: []string{},
			},
			filter:   types.TagPresentFilter{Tag: "ranked_ready"},
			expected: false,
		},
		{
			name:     "nil tags",
			fields:   types.SearchFields{},
			filter:   types.TagPresentFilter{Tag: "ranked_ready"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := passesTagPresentFilter(tt.fields, tt.filter)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPlayerPassesPoolFilters(t *testing.T) {
	tests := []struct {
		name     string
		fields   types.SearchFields
		pool     types.Pool
		expected bool
	}{
		{
			name: "passes all filters",
			fields: types.SearchFields{
				StringArgs: map[string]string{"region": "NA"},
				DoubleArgs: map[string]float64{"elo": 1500},
				Tags:       []string{"ranked_ready"},
			},
			pool: types.Pool{
				Name: "default",
				StringEqualsFilters: []types.StringEqualsFilter{
					{Field: "region", Value: "NA"},
				},
				DoubleRangeFilters: []types.DoubleRangeFilter{
					{Field: "elo", Min: 1000, Max: 2000},
				},
				TagPresentFilters: []types.TagPresentFilter{
					{Tag: "ranked_ready"},
				},
			},
			expected: true,
		},
		{
			name: "fails string filter",
			fields: types.SearchFields{
				StringArgs: map[string]string{"region": "EU"},
				DoubleArgs: map[string]float64{"elo": 1500},
				Tags:       []string{"ranked_ready"},
			},
			pool: types.Pool{
				StringEqualsFilters: []types.StringEqualsFilter{
					{Field: "region", Value: "NA"},
				},
			},
			expected: false,
		},
		{
			name: "fails double filter",
			fields: types.SearchFields{
				StringArgs: map[string]string{"region": "NA"},
				DoubleArgs: map[string]float64{"elo": 500},
				Tags:       []string{"ranked_ready"},
			},
			pool: types.Pool{
				DoubleRangeFilters: []types.DoubleRangeFilter{
					{Field: "elo", Min: 1000, Max: 2000},
				},
			},
			expected: false,
		},
		{
			name: "fails tag filter",
			fields: types.SearchFields{
				StringArgs: map[string]string{"region": "NA"},
				DoubleArgs: map[string]float64{"elo": 1500},
				Tags:       []string{"verified"},
			},
			pool: types.Pool{
				TagPresentFilters: []types.TagPresentFilter{
					{Tag: "ranked_ready"},
				},
			},
			expected: false,
		},
		{
			name: "no filters - passes by default",
			fields: types.SearchFields{
				StringArgs: map[string]string{"region": "NA"},
			},
			pool:     types.Pool{Name: "default"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PlayerPassesPoolFilters(tt.fields, tt.pool)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDerivePoolCounts(t *testing.T) {
	profile := &types.Profile{
		Name: "5v5-roles",
		Pools: []types.Pool{
			{
				Name: "tank",
				StringEqualsFilters: []types.StringEqualsFilter{
					{Field: "role", Value: "tank"},
				},
			},
			{
				Name: "dps",
				StringEqualsFilters: []types.StringEqualsFilter{
					{Field: "role", Value: "dps"},
				},
			},
			{
				Name: "support",
				StringEqualsFilters: []types.StringEqualsFilter{
					{Field: "role", Value: "support"},
				},
			},
		},
	}

	tests := []struct {
		name     string
		ticket   *types.Ticket
		expected map[string]int
	}{
		{
			name: "single tank player",
			ticket: &types.Ticket{
				Players: []types.PlayerInfo{
					{
						PlayerID: "player1",
						SearchFields: types.SearchFields{
							StringArgs: map[string]string{"role": "tank"},
						},
					},
				},
			},
			expected: map[string]int{"tank": 1},
		},
		{
			name: "mixed party - tank and dps",
			ticket: &types.Ticket{
				Players: []types.PlayerInfo{
					{
						PlayerID: "player1",
						SearchFields: types.SearchFields{
							StringArgs: map[string]string{"role": "tank"},
						},
					},
					{
						PlayerID: "player2",
						SearchFields: types.SearchFields{
							StringArgs: map[string]string{"role": "dps"},
						},
					},
				},
			},
			expected: map[string]int{"tank": 1, "dps": 1},
		},
		{
			name: "no matching pools",
			ticket: &types.Ticket{
				Players: []types.PlayerInfo{
					{
						PlayerID: "player1",
						SearchFields: types.SearchFields{
							StringArgs: map[string]string{"role": "unknown"},
						},
					},
				},
			},
			expected: map[string]int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DerivePoolCounts(tt.ticket, profile)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilterCandidates(t *testing.T) {
	profile := &types.Profile{
		Name:      "1v1-ranked",
		TeamCount: 2,
		TeamSize:  1,
		Pools: []types.Pool{
			{
				Name: "default",
				StringEqualsFilters: []types.StringEqualsFilter{
					{Field: "region", Value: "NA"},
				},
			},
		},
	}

	now := time.Now()
	tickets := []*types.Ticket{
		{
			ID:               "ticket1",
			MatchProfileName: "1v1-ranked",
			Players: []types.PlayerInfo{
				{
					PlayerID: "player1",
					SearchFields: types.SearchFields{
						StringArgs: map[string]string{"region": "NA"},
					},
				},
			},
			CreatedAt: now,
		},
		{
			ID:               "ticket2",
			MatchProfileName: "1v1-ranked",
			Players: []types.PlayerInfo{
				{
					PlayerID: "player2",
					SearchFields: types.SearchFields{
						StringArgs: map[string]string{"region": "EU"}, // Wrong region
					},
				},
			},
			CreatedAt: now,
		},
		{
			ID:               "ticket3",
			MatchProfileName: "1v1-ranked",
			Players: []types.PlayerInfo{
				{
					PlayerID: "player3",
					SearchFields: types.SearchFields{
						StringArgs: map[string]string{"region": "NA"},
					},
				},
				{
					PlayerID: "player4",
					SearchFields: types.SearchFields{
						StringArgs: map[string]string{"region": "NA"},
					},
				},
			}, // Party size > team size
			CreatedAt: now,
		},
	}

	result := FilterCandidates(tickets, profile)

	require.Len(t, result, 1)
	assert.Equal(t, "ticket1", result[0].ID)
}

func TestFilterBackfillCandidates(t *testing.T) {
	profile := &types.Profile{
		Name:      "5v5-roles",
		TeamCount: 2,
		TeamSize:  5,
		Pools: []types.Pool{
			{
				Name: "tank",
				StringEqualsFilters: []types.StringEqualsFilter{
					{Field: "role", Value: "tank"},
				},
			},
			{
				Name: "dps",
				StringEqualsFilters: []types.StringEqualsFilter{
					{Field: "role", Value: "dps"},
				},
			},
		},
	}

	slotsNeeded := []types.SlotNeeded{
		{PoolName: "tank", Count: 1},
	}

	now := time.Now()
	tickets := []*types.Ticket{
		{
			ID:               "ticket1",
			AllowBackfill:    true,
			MatchProfileName: "5v5-roles",
			Players: []types.PlayerInfo{
				{
					PlayerID: "player1",
					SearchFields: types.SearchFields{
						StringArgs: map[string]string{"role": "tank"},
					},
				},
			},
			CreatedAt: now,
		},
		{
			ID:               "ticket2",
			AllowBackfill:    false, // Not backfill eligible
			MatchProfileName: "5v5-roles",
			Players: []types.PlayerInfo{
				{
					PlayerID: "player2",
					SearchFields: types.SearchFields{
						StringArgs: map[string]string{"role": "tank"},
					},
				},
			},
			CreatedAt: now,
		},
		{
			ID:               "ticket3",
			AllowBackfill:    true,
			MatchProfileName: "5v5-roles",
			Players: []types.PlayerInfo{
				{
					PlayerID: "player3",
					SearchFields: types.SearchFields{
						StringArgs: map[string]string{"role": "dps"}, // Doesn't match needed pool
					},
				},
			},
			CreatedAt: now,
		},
	}

	result := FilterBackfillCandidates(tickets, profile, slotsNeeded)

	require.Len(t, result, 1)
	assert.Equal(t, "ticket1", result[0].ID)
}

func TestGetMaxTeamSize(t *testing.T) {
	tests := []struct {
		name     string
		profile  *types.Profile
		expected int
	}{
		{
			name: "symmetric profile",
			profile: &types.Profile{
				TeamCount: 2,
				TeamSize:  5,
			},
			expected: 5,
		},
		{
			name: "asymmetric profile",
			profile: &types.Profile{
				Teams: []types.TeamDefinition{
					{Name: "attackers", Size: 5},
					{Name: "defenders", Size: 3},
				},
			},
			expected: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getMaxTeamSize(tt.profile)
			assert.Equal(t, tt.expected, result)
		})
	}
}
