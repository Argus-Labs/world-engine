package algorithm

import (
	"testing"
	"time"
)

// mockTicket implements Ticket interface for testing.
type mockTicket struct {
	id            string
	createdAt     time.Time
	poolCounts    map[string]int
	players       int
	firstPlayerID string
}

func (t *mockTicket) GetID() string                 { return t.id }
func (t *mockTicket) GetCreatedAt() time.Time       { return t.createdAt }
func (t *mockTicket) GetPoolCounts() map[string]int { return t.poolCounts }
func (t *mockTicket) PlayerCount() int              { return t.players }
func (t *mockTicket) GetFirstPlayerID() string      { return t.firstPlayerID }

// mockProfile implements Profile interface for testing.
type mockProfile struct {
	teamCount   int
	teamSize    int
	teamMinSize int
	teamNames   []string
	composition map[string]int
}

func (p *mockProfile) GetTeamCount() int                                  { return p.teamCount }
func (p *mockProfile) GetTeamSize(teamIndex int) int                      { return p.teamSize }
func (p *mockProfile) GetTeamMinSize(teamIndex int) int                   { return p.teamMinSize }
func (p *mockProfile) GetTeamName(teamIndex int) string                   { return p.teamNames[teamIndex] }
func (p *mockProfile) GetTeamCompositionMap(teamIndex int) map[string]int { return p.composition }
func (p *mockProfile) HasRoles() bool                                     { return len(p.composition) > 0 }

// asymmetricProfile implements Profile for asymmetric team tests.
type asymmetricProfile struct {
	teams []teamDef
}

type teamDef struct {
	name        string
	size        int
	minSize     int
	composition map[string]int
}

func (p *asymmetricProfile) GetTeamCount() int        { return len(p.teams) }
func (p *asymmetricProfile) GetTeamSize(i int) int    { return p.teams[i].size }
func (p *asymmetricProfile) GetTeamMinSize(i int) int { return p.teams[i].minSize }
func (p *asymmetricProfile) GetTeamName(i int) string { return p.teams[i].name }
func (p *asymmetricProfile) GetTeamCompositionMap(i int) map[string]int {
	return p.teams[i].composition
}
func (p *asymmetricProfile) HasRoles() bool {
	for _, t := range p.teams {
		if len(t.composition) > 0 {
			return true
		}
	}
	return false
}

// helper to create tickets
func ticket(id string, createdAt time.Time, pools map[string]int, players int) *mockTicket {
	return &mockTicket{
		id:            id,
		createdAt:     createdAt,
		poolCounts:    pools,
		players:       players,
		firstPlayerID: "player-" + id, // use ticket ID as player ID for deterministic sorting
	}
}

// helper to convert mockTickets to Ticket slice
func tickets(mocks ...*mockTicket) []Ticket {
	result := make([]Ticket, len(mocks))
	for i, m := range mocks {
		result[i] = m
	}
	return result
}

func TestBasic(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name        string
		candidates  []Ticket
		profile     Profile
		wantSuccess bool
		wantTeams   int // number of assignments expected
	}{
		{
			name:        "empty candidates",
			candidates:  nil,
			profile:     &mockProfile{teamCount: 2, teamSize: 5, teamMinSize: 5, teamNames: []string{"team_1", "team_2"}},
			wantSuccess: false,
			wantTeams:   0,
		},
		{
			name: "single ticket single team",
			candidates: tickets(
				ticket("t1", now, map[string]int{"default": 1}, 1),
			),
			profile:     &mockProfile{teamCount: 1, teamSize: 1, teamMinSize: 1, teamNames: []string{"team_1"}},
			wantSuccess: true,
			wantTeams:   1,
		},
		{
			name: "exact fit",
			candidates: tickets(
				ticket("t1", now, map[string]int{"default": 1}, 1),
				ticket("t2", now, map[string]int{"default": 1}, 1),
				ticket("t3", now, map[string]int{"default": 1}, 1),
				ticket("t4", now, map[string]int{"default": 1}, 1),
				ticket("t5", now, map[string]int{"default": 1}, 1),
				ticket("t6", now, map[string]int{"default": 1}, 1),
				ticket("t7", now, map[string]int{"default": 1}, 1),
				ticket("t8", now, map[string]int{"default": 1}, 1),
				ticket("t9", now, map[string]int{"default": 1}, 1),
				ticket("t10", now, map[string]int{"default": 1}, 1),
			),
			profile:     &mockProfile{teamCount: 2, teamSize: 5, teamMinSize: 5, teamNames: []string{"team_1", "team_2"}},
			wantSuccess: true,
			wantTeams:   10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := NewInput(tt.candidates, tt.profile, now)
			got := Run(input)

			if got.Success != tt.wantSuccess {
				t.Errorf("Success = %v, want %v", got.Success, tt.wantSuccess)
			}
			if len(got.Assignments) != tt.wantTeams {
				t.Errorf("Assignments count = %d, want %d", len(got.Assignments), tt.wantTeams)
			}
		})
	}
}

func TestTeamSize(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name        string
		candidates  []Ticket
		profile     Profile
		wantSuccess bool
		wantCount   int
	}{
		{
			name: "not enough candidates",
			candidates: tickets(
				ticket("t1", now, map[string]int{"default": 1}, 1),
				ticket("t2", now, map[string]int{"default": 1}, 1),
				ticket("t3", now, map[string]int{"default": 1}, 1),
			),
			profile:     &mockProfile{teamCount: 2, teamSize: 5, teamMinSize: 5, teamNames: []string{"team_1", "team_2"}},
			wantSuccess: false,
			wantCount:   0,
		},
		{
			name: "too many candidates picks oldest",
			candidates: tickets(
				ticket("t1", now.Add(-5*time.Minute), map[string]int{"default": 1}, 1),
				ticket("t2", now.Add(-4*time.Minute), map[string]int{"default": 1}, 1),
				ticket("t3", now.Add(-3*time.Minute), map[string]int{"default": 1}, 1),
				ticket("t4", now.Add(-2*time.Minute), map[string]int{"default": 1}, 1),
				ticket("t5", now.Add(-1*time.Minute), map[string]int{"default": 1}, 1), // newest, should be excluded
			),
			profile:     &mockProfile{teamCount: 2, teamSize: 2, teamMinSize: 2, teamNames: []string{"team_1", "team_2"}},
			wantSuccess: true,
			wantCount:   4, // only 4 needed
		},
		{
			name: "min max size difference",
			candidates: tickets(
				ticket("t1", now, map[string]int{"default": 1}, 1),
				ticket("t2", now, map[string]int{"default": 1}, 1),
				ticket("t3", now, map[string]int{"default": 1}, 1),
			),
			profile:     &mockProfile{teamCount: 2, teamSize: 3, teamMinSize: 1, teamNames: []string{"team_1", "team_2"}},
			wantSuccess: true,
			wantCount:   2, // completes when each team has minSize (1), so 2 tickets needed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := NewInput(tt.candidates, tt.profile, now)
			got := Run(input)

			if got.Success != tt.wantSuccess {
				t.Errorf("Success = %v, want %v", got.Success, tt.wantSuccess)
			}
			if len(got.Assignments) != tt.wantCount {
				t.Errorf("Assignments count = %d, want %d", len(got.Assignments), tt.wantCount)
			}
		})
	}
}

func TestRoles(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name        string
		candidates  []Ticket
		profile     Profile
		wantSuccess bool
		wantCount   int
	}{
		{
			name: "role based assignment",
			candidates: tickets(
				// Team 1
				ticket("tank1", now, map[string]int{"tank": 1}, 1),
				ticket("dps1", now, map[string]int{"dps": 1}, 1),
				ticket("dps2", now, map[string]int{"dps": 1}, 1),
				ticket("dps3", now, map[string]int{"dps": 1}, 1),
				ticket("healer1", now, map[string]int{"healer": 1}, 1),
				// Team 2
				ticket("tank2", now, map[string]int{"tank": 1}, 1),
				ticket("dps4", now, map[string]int{"dps": 1}, 1),
				ticket("dps5", now, map[string]int{"dps": 1}, 1),
				ticket("dps6", now, map[string]int{"dps": 1}, 1),
				ticket("healer2", now, map[string]int{"healer": 1}, 1),
			),
			profile: &mockProfile{
				teamCount:   2,
				teamSize:    5,
				teamMinSize: 5,
				teamNames:   []string{"team_1", "team_2"},
				composition: map[string]int{"tank": 1, "dps": 3, "healer": 1},
			},
			wantSuccess: true,
			wantCount:   10,
		},
		{
			name: "missing roles",
			candidates: tickets(
				ticket("dps1", now, map[string]int{"dps": 1}, 1),
				ticket("dps2", now, map[string]int{"dps": 1}, 1),
				ticket("dps3", now, map[string]int{"dps": 1}, 1),
				ticket("dps4", now, map[string]int{"dps": 1}, 1),
				ticket("dps5", now, map[string]int{"dps": 1}, 1),
				// No tanks or healers
			),
			profile: &mockProfile{
				teamCount:   1,
				teamSize:    5,
				teamMinSize: 5,
				teamNames:   []string{"team_1"},
				composition: map[string]int{"tank": 1, "dps": 3, "healer": 1},
			},
			wantSuccess: false,
			wantCount:   0,
		},
		{
			name: "mixed pool counts",
			candidates: tickets(
				// Flex player: counts as both tank AND dps (party of 1 with multiple roles)
				ticket("flex1", now, map[string]int{"tank": 1, "dps": 1}, 1),
				ticket("dps1", now, map[string]int{"dps": 1}, 1),
				ticket("healer1", now, map[string]int{"healer": 1}, 1),
			),
			profile: &mockProfile{
				teamCount:   1,
				teamSize:    3,
				teamMinSize: 3,
				teamNames:   []string{"team_1"},
				composition: map[string]int{"tank": 1, "dps": 2, "healer": 1}, // flex provides tank:1 + dps:1, dps1 provides dps:1
			},
			wantSuccess: true,
			wantCount:   3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := NewInput(tt.candidates, tt.profile, now)
			got := Run(input)

			if got.Success != tt.wantSuccess {
				t.Errorf("Success = %v, want %v", got.Success, tt.wantSuccess)
			}
			if len(got.Assignments) != tt.wantCount {
				t.Errorf("Assignments count = %d, want %d", len(got.Assignments), tt.wantCount)
			}
		})
	}
}

func TestBackfill(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name        string
		candidates  []Ticket
		slots       []SlotNeeded
		wantSuccess bool
		wantCount   int
	}{
		{
			name: "basic backfill",
			candidates: tickets(
				ticket("t1", now, map[string]int{"default": 1}, 1),
				ticket("t2", now, map[string]int{"default": 1}, 1),
				ticket("t3", now, map[string]int{"default": 1}, 1),
			),
			slots:       []SlotNeeded{{PoolName: "default", Count: 2}},
			wantSuccess: true,
			wantCount:   2,
		},
		{
			name: "backfill with roles",
			candidates: tickets(
				ticket("tank1", now, map[string]int{"tank": 1}, 1),
				ticket("healer1", now, map[string]int{"healer": 1}, 1),
				ticket("dps1", now, map[string]int{"dps": 1}, 1),
			),
			slots:       []SlotNeeded{{PoolName: "tank", Count: 1}, {PoolName: "healer", Count: 1}},
			wantSuccess: true,
			wantCount:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := NewBackfillInput(tt.candidates, tt.slots, now)
			got := Run(input)

			if got.Success != tt.wantSuccess {
				t.Errorf("Success = %v, want %v", got.Success, tt.wantSuccess)
			}
			if len(got.Assignments) != tt.wantCount {
				t.Errorf("Assignments count = %d, want %d", len(got.Assignments), tt.wantCount)
			}
		})
	}
}

func TestPriority(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name       string
		candidates []Ticket
		profile    Profile
		wantIDs    []string // expected ticket IDs in result (oldest first)
	}{
		{
			name: "oldest tickets first",
			candidates: tickets(
				ticket("new1", now.Add(-1*time.Minute), map[string]int{"default": 1}, 1),
				ticket("old1", now.Add(-10*time.Minute), map[string]int{"default": 1}, 1),
				ticket("new2", now.Add(-2*time.Minute), map[string]int{"default": 1}, 1),
				ticket("old2", now.Add(-9*time.Minute), map[string]int{"default": 1}, 1),
				ticket("newest", now, map[string]int{"default": 1}, 1),
			),
			profile: &mockProfile{teamCount: 1, teamSize: 3, teamMinSize: 3, teamNames: []string{"team_1"}},
			wantIDs: []string{"old1", "old2", "new2"}, // 3 oldest
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := NewInput(tt.candidates, tt.profile, now)
			got := Run(input)

			if !got.Success {
				t.Fatal("expected Success = true")
			}

			// Check that assigned tickets are the oldest ones
			gotIDs := make(map[string]bool)
			for _, a := range got.Assignments {
				gotIDs[a.Ticket.GetID()] = true
			}

			for _, wantID := range tt.wantIDs {
				if !gotIDs[wantID] {
					t.Errorf("expected ticket %s to be assigned", wantID)
				}
			}
		})
	}
}

func TestAsymmetric(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name        string
		candidates  []Ticket
		profile     Profile
		wantSuccess bool
		wantCount   int
	}{
		{
			name: "asymmetric teams different sizes",
			candidates: tickets(
				ticket("t1", now, map[string]int{"default": 1}, 1),
				ticket("t2", now, map[string]int{"default": 1}, 1),
				ticket("t3", now, map[string]int{"default": 1}, 1),
				ticket("t4", now, map[string]int{"default": 1}, 1),
				ticket("t5", now, map[string]int{"default": 1}, 1),
			),
			profile: &asymmetricProfile{
				teams: []teamDef{
					{name: "small_team", size: 2, minSize: 2},
					{name: "large_team", size: 3, minSize: 3},
				},
			},
			wantSuccess: true,
			wantCount:   5,
		},
		{
			name: "three plus teams",
			candidates: tickets(
				ticket("t1", now, map[string]int{"default": 1}, 1),
				ticket("t2", now, map[string]int{"default": 1}, 1),
				ticket("t3", now, map[string]int{"default": 1}, 1),
				ticket("t4", now, map[string]int{"default": 1}, 1),
				ticket("t5", now, map[string]int{"default": 1}, 1),
				ticket("t6", now, map[string]int{"default": 1}, 1),
			),
			profile: &asymmetricProfile{
				teams: []teamDef{
					{name: "team_a", size: 2, minSize: 2},
					{name: "team_b", size: 2, minSize: 2},
					{name: "team_c", size: 2, minSize: 2},
				},
			},
			wantSuccess: true,
			wantCount:   6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := NewInput(tt.candidates, tt.profile, now)
			got := Run(input)

			if got.Success != tt.wantSuccess {
				t.Errorf("Success = %v, want %v", got.Success, tt.wantSuccess)
			}
			if len(got.Assignments) != tt.wantCount {
				t.Errorf("Assignments count = %d, want %d", len(got.Assignments), tt.wantCount)
			}
		})
	}
}

func TestDebug(t *testing.T) {
	t.Parallel()

	now := time.Now()
	candidates := tickets(
		ticket("t1", now, map[string]int{"default": 1}, 1),
		ticket("t2", now, map[string]int{"default": 1}, 1),
	)
	profile := &mockProfile{teamCount: 1, teamSize: 2, teamMinSize: 2, teamNames: []string{"team_1"}}

	tests := []struct {
		name          string
		debug         bool
		wantStatsZero bool
	}{
		{
			name:          "debug off",
			debug:         false,
			wantStatsZero: true,
		},
		{
			name:          "debug on",
			debug:         true,
			wantStatsZero: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := NewInput(candidates, profile, now)
			if tt.debug {
				input = input.WithDebug()
			}

			got := Run(input)

			if !got.Success {
				t.Fatal("expected Success = true")
			}

			statsIsZero := got.Stats.CandidatesConsidered == 0 &&
				got.Stats.StatesExplored == 0 &&
				got.Stats.Duration == 0

			if statsIsZero != tt.wantStatsZero {
				t.Errorf("Stats zero = %v, want %v. Stats: %+v", statsIsZero, tt.wantStatsZero, got.Stats)
			}
		})
	}
}

func TestDeterministic5v5Roles(t *testing.T) {
	t.Parallel()

	now := time.Now()

	// Create 10 tickets with same timestamp (simulating same tick)
	// Sorted alphabetically by player ID
	candidates := tickets(
		ticket("dps-daisy", now, map[string]int{"dps": 1}, 1),
		ticket("dps-dan", now, map[string]int{"dps": 1}, 1),
		ticket("dps-dave", now, map[string]int{"dps": 1}, 1),
		ticket("dps-derek", now, map[string]int{"dps": 1}, 1),
		ticket("dps-diana", now, map[string]int{"dps": 1}, 1),
		ticket("dps-dora", now, map[string]int{"dps": 1}, 1),
		ticket("healer-hank", now, map[string]int{"support": 1}, 1),
		ticket("healer-helen", now, map[string]int{"support": 1}, 1),
		ticket("tank-tina", now, map[string]int{"tank": 1}, 1),
		ticket("tank-tom", now, map[string]int{"tank": 1}, 1),
	)

	profile := &mockProfile{
		teamCount:   2,
		teamSize:    5,
		teamMinSize: 5,
		teamNames:   []string{"team_1", "team_2"},
		composition: map[string]int{"tank": 1, "dps": 3, "support": 1},
	}

	// Run multiple times to verify determinism
	var firstResult []Assignment
	for i := 0; i < 5; i++ {
		input := NewInput(candidates, profile, now)
		got := Run(input)

		if !got.Success {
			t.Fatalf("Run %d: expected Success = true", i)
		}

		if len(got.Assignments) != 10 {
			t.Fatalf("Run %d: expected 10 assignments, got %d", i, len(got.Assignments))
		}

		if i == 0 {
			firstResult = got.Assignments
			t.Logf("First run assignments:")
			for _, a := range got.Assignments {
				t.Logf("  %s -> %s", a.Ticket.GetFirstPlayerID(), a.TeamName)
			}
		} else {
			// Compare with first run
			for j, a := range got.Assignments {
				if a.Ticket.GetID() != firstResult[j].Ticket.GetID() ||
					a.TeamIndex != firstResult[j].TeamIndex {
					t.Errorf("Run %d: assignment %d differs from first run", i, j)
					t.Errorf("  First: %s -> team_%d", firstResult[j].Ticket.GetFirstPlayerID(), firstResult[j].TeamIndex)
					t.Errorf("  This:  %s -> team_%d", a.Ticket.GetFirstPlayerID(), a.TeamIndex)
				}
			}
		}
	}

	// Check expected team assignments
	// With alphabetical sorting and greedy-like assignment:
	// team_1 should get: daisy, dan, dave (first 3 dps), hank (first support), tina (first tank)
	// team_2 should get: derek, diana, dora (next 3 dps), helen (next support), tom (next tank)
	expectedTeam1 := map[string]bool{
		"player-dps-daisy":   true,
		"player-dps-dan":     true,
		"player-dps-dave":    true,
		"player-healer-hank": true,
		"player-tank-tina":   true,
	}
	expectedTeam2 := map[string]bool{
		"player-dps-derek":    true,
		"player-dps-diana":    true,
		"player-dps-dora":     true,
		"player-healer-helen": true,
		"player-tank-tom":     true,
	}

	for _, a := range firstResult {
		playerID := a.Ticket.GetFirstPlayerID()
		if a.TeamIndex == 0 {
			if !expectedTeam1[playerID] {
				t.Errorf("Unexpected player %s in team_1, expected in team_2", playerID)
			}
		} else {
			if !expectedTeam2[playerID] {
				t.Errorf("Unexpected player %s in team_2, expected in team_1", playerID)
			}
		}
	}
}
