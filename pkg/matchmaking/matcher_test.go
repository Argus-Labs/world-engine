package matchmaking

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/matchmaking/types"
)

func TestRunMatchmaking_1v1(t *testing.T) {
	// Simple 1v1 profile
	profile := &types.Profile{
		Name:      "1v1-ranked",
		TeamCount: 2,
		TeamSize:  1,
		Pools: []types.Pool{
			{Name: "default"},
		},
	}

	now := time.Now()
	candidates := []*types.Ticket{
		{
			ID:        "ticket1",
			CreatedAt: now.Add(-2 * time.Second),
			Players: []types.PlayerInfo{
				{PlayerID: "player1"},
			},
			PoolCounts: map[string]int{"default": 1},
		},
		{
			ID:        "ticket2",
			CreatedAt: now.Add(-1 * time.Second),
			Players: []types.PlayerInfo{
				{PlayerID: "player2"},
			},
			PoolCounts: map[string]int{"default": 1},
		},
	}

	result := RunMatchmaking(candidates, profile, now)

	require.True(t, result.Success)
	assert.Len(t, result.Assignments, 2)

	// Verify both tickets assigned to different teams
	teamAssignments := make(map[int]bool)
	for _, a := range result.Assignments {
		teamAssignments[a.TeamIndex] = true
	}
	assert.Len(t, teamAssignments, 2, "both teams should have assignments")
}

func TestRunMatchmaking_2v2(t *testing.T) {
	profile := &types.Profile{
		Name:      "2v2-competitive",
		TeamCount: 2,
		TeamSize:  2,
		Pools: []types.Pool{
			{Name: "default"},
		},
	}

	now := time.Now()
	candidates := []*types.Ticket{
		{
			ID:        "ticket1",
			CreatedAt: now.Add(-4 * time.Second),
			Players: []types.PlayerInfo{
				{PlayerID: "player1"},
			},
			PoolCounts: map[string]int{"default": 1},
		},
		{
			ID:        "ticket2",
			CreatedAt: now.Add(-3 * time.Second),
			Players: []types.PlayerInfo{
				{PlayerID: "player2"},
			},
			PoolCounts: map[string]int{"default": 1},
		},
		{
			ID:        "ticket3",
			CreatedAt: now.Add(-2 * time.Second),
			Players: []types.PlayerInfo{
				{PlayerID: "player3"},
			},
			PoolCounts: map[string]int{"default": 1},
		},
		{
			ID:        "ticket4",
			CreatedAt: now.Add(-1 * time.Second),
			Players: []types.PlayerInfo{
				{PlayerID: "player4"},
			},
			PoolCounts: map[string]int{"default": 1},
		},
	}

	result := RunMatchmaking(candidates, profile, now)

	require.True(t, result.Success)
	assert.Len(t, result.Assignments, 4)

	// Count players per team
	teamCounts := make(map[int]int)
	for _, a := range result.Assignments {
		teamCounts[a.TeamIndex]++
	}
	assert.Equal(t, 2, teamCounts[0], "team 0 should have 2 players")
	assert.Equal(t, 2, teamCounts[1], "team 1 should have 2 players")
}

func TestRunMatchmaking_InsufficientPlayers(t *testing.T) {
	profile := &types.Profile{
		Name:      "1v1-ranked",
		TeamCount: 2,
		TeamSize:  1,
		Pools: []types.Pool{
			{Name: "default"},
		},
	}

	now := time.Now()
	// Only 1 player - not enough for 1v1
	candidates := []*types.Ticket{
		{
			ID:        "ticket1",
			CreatedAt: now,
			Players: []types.PlayerInfo{
				{PlayerID: "player1"},
			},
			PoolCounts: map[string]int{"default": 1},
		},
	}

	result := RunMatchmaking(candidates, profile, now)

	assert.False(t, result.Success)
}

func TestRunMatchmaking_RoleBased(t *testing.T) {
	// 5v5 with role requirements: 1 tank, 3 dps, 1 support per team
	profile := &types.Profile{
		Name:      "5v5-roles",
		TeamCount: 2,
		TeamSize:  5,
		Pools: []types.Pool{
			{Name: "tank"},
			{Name: "dps"},
			{Name: "support"},
		},
		TeamComposition: []types.PoolRequirement{
			{Pool: "tank", Count: 1},
			{Pool: "dps", Count: 3},
			{Pool: "support", Count: 1},
		},
	}

	now := time.Now()

	// Create 10 players: 2 tanks, 6 dps, 2 supports
	candidates := []*types.Ticket{
		// Tanks
		{ID: "tank1", CreatedAt: now.Add(-10 * time.Second), Players: []types.PlayerInfo{{PlayerID: "p1"}}, PoolCounts: map[string]int{"tank": 1}},
		{ID: "tank2", CreatedAt: now.Add(-9 * time.Second), Players: []types.PlayerInfo{{PlayerID: "p2"}}, PoolCounts: map[string]int{"tank": 1}},
		// DPS
		{ID: "dps1", CreatedAt: now.Add(-8 * time.Second), Players: []types.PlayerInfo{{PlayerID: "p3"}}, PoolCounts: map[string]int{"dps": 1}},
		{ID: "dps2", CreatedAt: now.Add(-7 * time.Second), Players: []types.PlayerInfo{{PlayerID: "p4"}}, PoolCounts: map[string]int{"dps": 1}},
		{ID: "dps3", CreatedAt: now.Add(-6 * time.Second), Players: []types.PlayerInfo{{PlayerID: "p5"}}, PoolCounts: map[string]int{"dps": 1}},
		{ID: "dps4", CreatedAt: now.Add(-5 * time.Second), Players: []types.PlayerInfo{{PlayerID: "p6"}}, PoolCounts: map[string]int{"dps": 1}},
		{ID: "dps5", CreatedAt: now.Add(-4 * time.Second), Players: []types.PlayerInfo{{PlayerID: "p7"}}, PoolCounts: map[string]int{"dps": 1}},
		{ID: "dps6", CreatedAt: now.Add(-3 * time.Second), Players: []types.PlayerInfo{{PlayerID: "p8"}}, PoolCounts: map[string]int{"dps": 1}},
		// Supports
		{ID: "support1", CreatedAt: now.Add(-2 * time.Second), Players: []types.PlayerInfo{{PlayerID: "p9"}}, PoolCounts: map[string]int{"support": 1}},
		{ID: "support2", CreatedAt: now.Add(-1 * time.Second), Players: []types.PlayerInfo{{PlayerID: "p10"}}, PoolCounts: map[string]int{"support": 1}},
	}

	result := RunMatchmaking(candidates, profile, now)

	require.True(t, result.Success)
	assert.Len(t, result.Assignments, 10)

	// Verify team composition
	for teamIdx := 0; teamIdx < 2; teamIdx++ {
		poolCounts := make(map[string]int)
		for _, a := range result.Assignments {
			if a.TeamIndex == teamIdx {
				for pool := range a.Ticket.PoolCounts {
					poolCounts[pool]++
				}
			}
		}
		assert.Equal(t, 1, poolCounts["tank"], "team %d should have 1 tank", teamIdx)
		assert.Equal(t, 3, poolCounts["dps"], "team %d should have 3 dps", teamIdx)
		assert.Equal(t, 1, poolCounts["support"], "team %d should have 1 support", teamIdx)
	}
}

func TestRunBackfillMatchmaking(t *testing.T) {
	now := time.Now()

	// Need 2 DPS players
	slotsNeeded := []types.SlotNeeded{
		{PoolName: "dps", Count: 2},
	}

	candidates := []*types.Ticket{
		{
			ID:            "ticket1",
			AllowBackfill: true,
			CreatedAt:     now.Add(-2 * time.Second),
			Players: []types.PlayerInfo{
				{PlayerID: "player1"},
			},
			PoolCounts: map[string]int{"dps": 1},
		},
		{
			ID:            "ticket2",
			AllowBackfill: true,
			CreatedAt:     now.Add(-1 * time.Second),
			Players: []types.PlayerInfo{
				{PlayerID: "player2"},
			},
			PoolCounts: map[string]int{"dps": 1},
		},
	}

	result := RunBackfillMatchmaking(candidates, slotsNeeded, now)

	require.True(t, result.Success)
	assert.Len(t, result.Assignments, 2)
}

func TestRunBackfillMatchmaking_InsufficientPlayers(t *testing.T) {
	now := time.Now()

	// Need 3 players but only have 2
	slotsNeeded := []types.SlotNeeded{
		{PoolName: "dps", Count: 3},
	}

	candidates := []*types.Ticket{
		{
			ID:            "ticket1",
			AllowBackfill: true,
			CreatedAt:     now.Add(-2 * time.Second),
			Players: []types.PlayerInfo{
				{PlayerID: "player1"},
			},
			PoolCounts: map[string]int{"dps": 1},
		},
		{
			ID:            "ticket2",
			AllowBackfill: true,
			CreatedAt:     now.Add(-1 * time.Second),
			Players: []types.PlayerInfo{
				{PlayerID: "player2"},
			},
			PoolCounts: map[string]int{"dps": 1},
		},
	}

	result := RunBackfillMatchmaking(candidates, slotsNeeded, now)

	assert.False(t, result.Success)
}
