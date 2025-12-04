package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProfile_IsSymmetric(t *testing.T) {
	tests := []struct {
		name     string
		profile  Profile
		expected bool
	}{
		{
			name: "symmetric - no teams defined",
			profile: Profile{
				TeamCount: 2,
				TeamSize:  5,
			},
			expected: true,
		},
		{
			name: "asymmetric - teams defined",
			profile: Profile{
				Teams: []TeamDefinition{
					{Name: "attackers", Size: 5},
					{Name: "defenders", Size: 3},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.profile.IsSymmetric()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProfile_GetTeamCount(t *testing.T) {
	tests := []struct {
		name     string
		profile  Profile
		expected int
	}{
		{
			name: "symmetric",
			profile: Profile{
				TeamCount: 2,
				TeamSize:  5,
			},
			expected: 2,
		},
		{
			name: "asymmetric",
			profile: Profile{
				Teams: []TeamDefinition{
					{Name: "attackers", Size: 5},
					{Name: "defenders", Size: 3},
					{Name: "spectators", Size: 1},
				},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.profile.GetTeamCount()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProfile_GetTeamSize(t *testing.T) {
	tests := []struct {
		name      string
		profile   Profile
		teamIndex int
		expected  int
	}{
		{
			name: "symmetric - any team",
			profile: Profile{
				TeamCount: 2,
				TeamSize:  5,
			},
			teamIndex: 0,
			expected:  5,
		},
		{
			name: "asymmetric - first team",
			profile: Profile{
				Teams: []TeamDefinition{
					{Name: "attackers", Size: 5},
					{Name: "defenders", Size: 3},
				},
			},
			teamIndex: 0,
			expected:  5,
		},
		{
			name: "asymmetric - second team",
			profile: Profile{
				Teams: []TeamDefinition{
					{Name: "attackers", Size: 5},
					{Name: "defenders", Size: 3},
				},
			},
			teamIndex: 1,
			expected:  3,
		},
		{
			name: "asymmetric - invalid index",
			profile: Profile{
				Teams: []TeamDefinition{
					{Name: "attackers", Size: 5},
				},
			},
			teamIndex: 5,
			expected:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.profile.GetTeamSize(tt.teamIndex)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProfile_GetTeamMinSize(t *testing.T) {
	tests := []struct {
		name      string
		profile   Profile
		teamIndex int
		expected  int
	}{
		{
			name: "symmetric - with min size",
			profile: Profile{
				TeamCount:   2,
				TeamSize:    5,
				TeamMinSize: 3,
			},
			teamIndex: 0,
			expected:  3,
		},
		{
			name: "symmetric - no min size (defaults to team size)",
			profile: Profile{
				TeamCount: 2,
				TeamSize:  5,
			},
			teamIndex: 0,
			expected:  5,
		},
		{
			name: "asymmetric - fixed size",
			profile: Profile{
				Teams: []TeamDefinition{
					{Name: "attackers", Size: 5},
				},
			},
			teamIndex: 0,
			expected:  5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.profile.GetTeamMinSize(tt.teamIndex)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProfile_GetTeamName(t *testing.T) {
	tests := []struct {
		name      string
		profile   Profile
		teamIndex int
		expected  string
	}{
		{
			name: "symmetric - first team",
			profile: Profile{
				TeamCount: 2,
				TeamSize:  5,
			},
			teamIndex: 0,
			expected:  "team_1",
		},
		{
			name: "symmetric - second team",
			profile: Profile{
				TeamCount: 2,
				TeamSize:  5,
			},
			teamIndex: 1,
			expected:  "team_2",
		},
		{
			name: "asymmetric - named teams",
			profile: Profile{
				Teams: []TeamDefinition{
					{Name: "attackers", Size: 5},
					{Name: "defenders", Size: 3},
				},
			},
			teamIndex: 0,
			expected:  "attackers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.profile.GetTeamName(tt.teamIndex)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProfile_GetTeamComposition(t *testing.T) {
	tests := []struct {
		name      string
		profile   Profile
		teamIndex int
		expected  []PoolRequirement
	}{
		{
			name: "symmetric - with composition",
			profile: Profile{
				TeamCount: 2,
				TeamSize:  5,
				TeamComposition: []PoolRequirement{
					{Pool: "tank", Count: 1},
					{Pool: "dps", Count: 3},
					{Pool: "support", Count: 1},
				},
			},
			teamIndex: 0,
			expected: []PoolRequirement{
				{Pool: "tank", Count: 1},
				{Pool: "dps", Count: 3},
				{Pool: "support", Count: 1},
			},
		},
		{
			name: "symmetric - no composition",
			profile: Profile{
				TeamCount: 2,
				TeamSize:  5,
			},
			teamIndex: 0,
			expected:  nil,
		},
		{
			name: "asymmetric - team-specific composition",
			profile: Profile{
				Teams: []TeamDefinition{
					{
						Name: "attackers",
						Size: 3,
						Composition: []PoolRequirement{
							{Pool: "attacker", Count: 3},
						},
					},
				},
			},
			teamIndex: 0,
			expected: []PoolRequirement{
				{Pool: "attacker", Count: 3},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.profile.GetTeamComposition(tt.teamIndex)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProfile_HasRoles(t *testing.T) {
	tests := []struct {
		name     string
		profile  Profile
		expected bool
	}{
		{
			name: "symmetric - with roles",
			profile: Profile{
				TeamCount: 2,
				TeamSize:  5,
				TeamComposition: []PoolRequirement{
					{Pool: "tank", Count: 1},
				},
			},
			expected: true,
		},
		{
			name: "symmetric - no roles",
			profile: Profile{
				TeamCount: 2,
				TeamSize:  5,
			},
			expected: false,
		},
		{
			name: "asymmetric - with roles",
			profile: Profile{
				Teams: []TeamDefinition{
					{
						Name: "team1",
						Size: 3,
						Composition: []PoolRequirement{
							{Pool: "role1", Count: 1},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "asymmetric - no roles",
			profile: Profile{
				Teams: []TeamDefinition{
					{Name: "team1", Size: 3},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.profile.HasRoles()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProfile_GetTeamCompositionMap(t *testing.T) {
	profile := Profile{
		TeamCount: 2,
		TeamSize:  5,
		TeamComposition: []PoolRequirement{
			{Pool: "tank", Count: 1},
			{Pool: "dps", Count: 3},
			{Pool: "support", Count: 1},
		},
	}

	result := profile.GetTeamCompositionMap(0)

	assert.Equal(t, map[string]int{
		"tank":    1,
		"dps":     3,
		"support": 1,
	}, result)
}

func TestProfile_TotalPlayersNeeded(t *testing.T) {
	tests := []struct {
		name     string
		profile  Profile
		expected int
	}{
		{
			name: "symmetric 1v1",
			profile: Profile{
				TeamCount: 2,
				TeamSize:  1,
			},
			expected: 2,
		},
		{
			name: "symmetric 5v5",
			profile: Profile{
				TeamCount: 2,
				TeamSize:  5,
			},
			expected: 10,
		},
		{
			name: "battle royale 10 teams",
			profile: Profile{
				TeamCount: 10,
				TeamSize:  1,
			},
			expected: 10,
		},
		{
			name: "asymmetric",
			profile: Profile{
				Teams: []TeamDefinition{
					{Name: "attackers", Size: 5},
					{Name: "defenders", Size: 3},
				},
			},
			expected: 8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.profile.TotalPlayersNeeded()
			assert.Equal(t, tt.expected, result)
		})
	}
}
