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
		{
			name:        "backfill empty candidates",
			candidates:  nil,
			slots:       []SlotNeeded{{PoolName: "default", Count: 1}},
			wantSuccess: false,
			wantCount:   0,
		},
		{
			name: "backfill not enough candidates",
			candidates: tickets(
				ticket("t1", now, map[string]int{"default": 1}, 1),
			),
			slots:       []SlotNeeded{{PoolName: "default", Count: 3}},
			wantSuccess: false,
			wantCount:   0,
		},
		{
			name: "backfill missing role",
			candidates: tickets(
				ticket("dps1", now, map[string]int{"dps": 1}, 1),
				ticket("dps2", now, map[string]int{"dps": 1}, 1),
			),
			slots:       []SlotNeeded{{PoolName: "tank", Count: 1}},
			wantSuccess: false,
			wantCount:   0,
		},
		{
			name: "backfill 5v5 roles partial team",
			candidates: tickets(
				ticket("tank1", now, map[string]int{"tank": 1}, 1),
				ticket("dps1", now, map[string]int{"dps": 1}, 1),
				ticket("dps2", now, map[string]int{"dps": 1}, 1),
				ticket("healer1", now, map[string]int{"support": 1}, 1),
			),
			// Need 1 tank, 2 dps, 1 support to complete a team missing these roles
			slots:       []SlotNeeded{{PoolName: "tank", Count: 1}, {PoolName: "dps", Count: 2}, {PoolName: "support", Count: 1}},
			wantSuccess: true,
			wantCount:   4,
		},
		{
			name: "backfill large slot count",
			candidates: tickets(
				ticket("p1", now, map[string]int{"default": 1}, 1),
				ticket("p2", now, map[string]int{"default": 1}, 1),
				ticket("p3", now, map[string]int{"default": 1}, 1),
				ticket("p4", now, map[string]int{"default": 1}, 1),
				ticket("p5", now, map[string]int{"default": 1}, 1),
				ticket("p6", now, map[string]int{"default": 1}, 1),
				ticket("p7", now, map[string]int{"default": 1}, 1),
				ticket("p8", now, map[string]int{"default": 1}, 1),
			),
			slots:       []SlotNeeded{{PoolName: "default", Count: 8}},
			wantSuccess: true,
			wantCount:   8,
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

// TestBackfillDeterministic tests that backfill produces deterministic results.
func TestBackfillDeterministic(t *testing.T) {
	t.Parallel()

	now := time.Now()

	// Test 1: Simple backfill - oldest tickets first
	t.Run("oldest tickets first", func(t *testing.T) {
		t.Parallel()

		candidates := tickets(
			ticket("new1", now.Add(-1*time.Minute), map[string]int{"default": 1}, 1),
			ticket("old1", now.Add(-10*time.Minute), map[string]int{"default": 1}, 1),
			ticket("new2", now.Add(-2*time.Minute), map[string]int{"default": 1}, 1),
			ticket("old2", now.Add(-9*time.Minute), map[string]int{"default": 1}, 1),
		)
		slots := []SlotNeeded{{PoolName: "default", Count: 2}}

		// Run multiple times
		for i := 0; i < 10; i++ {
			input := NewBackfillInput(candidates, slots, now)
			got := Run(input)

			if !got.Success {
				t.Fatalf("Run %d: expected Success = true", i)
			}

			// Should always pick old1 and old2 (the two oldest)
			gotIDs := make(map[string]bool)
			for _, a := range got.Assignments {
				gotIDs[a.Ticket.GetID()] = true
			}

			if !gotIDs["old1"] || !gotIDs["old2"] {
				t.Errorf("Run %d: expected old1 and old2, got %v", i, gotIDs)
			}
		}
	})

	// Test 2: Role-based backfill determinism
	t.Run("role based determinism", func(t *testing.T) {
		t.Parallel()

		candidates := tickets(
			ticket("tank-tom", now.Add(0*time.Millisecond), map[string]int{"tank": 1}, 1),
			ticket("tank-tina", now.Add(10*time.Millisecond), map[string]int{"tank": 1}, 1),
			ticket("dps-dan", now.Add(20*time.Millisecond), map[string]int{"dps": 1}, 1),
			ticket("dps-diana", now.Add(30*time.Millisecond), map[string]int{"dps": 1}, 1),
			ticket("dps-dave", now.Add(40*time.Millisecond), map[string]int{"dps": 1}, 1),
			ticket("healer-helen", now.Add(50*time.Millisecond), map[string]int{"support": 1}, 1),
		)
		slots := []SlotNeeded{
			{PoolName: "tank", Count: 1},
			{PoolName: "dps", Count: 2},
			{PoolName: "support", Count: 1},
		}

		// Get first result
		input := NewBackfillInput(candidates, slots, now.Add(100*time.Millisecond))
		first := Run(input)

		if !first.Success {
			t.Fatal("expected Success = true")
		}

		if len(first.Assignments) != 4 {
			t.Fatalf("expected 4 assignments, got %d", len(first.Assignments))
		}

		// Run multiple times and verify same result
		for i := 0; i < 10; i++ {
			input := NewBackfillInput(candidates, slots, now.Add(100*time.Millisecond))
			got := Run(input)

			if len(got.Assignments) != len(first.Assignments) {
				t.Fatalf("Run %d: assignment count mismatch", i)
			}

			for j, a := range got.Assignments {
				if a.Ticket.GetID() != first.Assignments[j].Ticket.GetID() {
					t.Errorf("Run %d: assignment[%d] ticket = %s, expected %s",
						i, j, a.Ticket.GetID(), first.Assignments[j].Ticket.GetID())
				}
			}
		}

		// Log the deterministic result
		t.Log("Deterministic backfill assignments:")
		for _, a := range first.Assignments {
			t.Logf("  %s", a.Ticket.GetID())
		}
	})

	// Test 3: Large backfill (8 slots like 4-team-squads leaving)
	t.Run("large backfill 8 slots", func(t *testing.T) {
		t.Parallel()

		candidates := tickets(
			ticket("p1", now.Add(0*time.Millisecond), map[string]int{"default": 1}, 1),
			ticket("p2", now.Add(10*time.Millisecond), map[string]int{"default": 1}, 1),
			ticket("p3", now.Add(20*time.Millisecond), map[string]int{"default": 1}, 1),
			ticket("p4", now.Add(30*time.Millisecond), map[string]int{"default": 1}, 1),
			ticket("p5", now.Add(40*time.Millisecond), map[string]int{"default": 1}, 1),
			ticket("p6", now.Add(50*time.Millisecond), map[string]int{"default": 1}, 1),
			ticket("p7", now.Add(60*time.Millisecond), map[string]int{"default": 1}, 1),
			ticket("p8", now.Add(70*time.Millisecond), map[string]int{"default": 1}, 1),
			ticket("p9", now.Add(80*time.Millisecond), map[string]int{"default": 1}, 1),  // extra
			ticket("p10", now.Add(90*time.Millisecond), map[string]int{"default": 1}, 1), // extra
		)
		slots := []SlotNeeded{{PoolName: "default", Count: 8}}

		// Get first result
		input := NewBackfillInput(candidates, slots, now.Add(100*time.Millisecond))
		first := Run(input)

		if !first.Success {
			t.Fatal("expected Success = true")
		}

		if len(first.Assignments) != 8 {
			t.Fatalf("expected 8 assignments, got %d", len(first.Assignments))
		}

		// Verify oldest 8 are picked (p1-p8, not p9 or p10)
		for _, a := range first.Assignments {
			id := a.Ticket.GetID()
			if id == "p9" || id == "p10" {
				t.Errorf("should not include %s (too new)", id)
			}
		}

		// Run multiple times and verify same result
		for i := 0; i < 10; i++ {
			input := NewBackfillInput(candidates, slots, now.Add(100*time.Millisecond))
			got := Run(input)

			for j, a := range got.Assignments {
				if a.Ticket.GetID() != first.Assignments[j].Ticket.GetID() {
					t.Errorf("Run %d: assignment[%d] ticket = %s, expected %s",
						i, j, a.Ticket.GetID(), first.Assignments[j].Ticket.GetID())
				}
			}
		}
	})
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

	// Create 10 tickets in the EXACT order as the demo (same as demo-matchmaking-lobby)
	// Each ticket created 10ms apart to simulate sequential creation
	candidates := tickets(
		ticket("tank-tom", now.Add(0*time.Millisecond), map[string]int{"tank": 1}, 1),
		ticket("dps-dan", now.Add(10*time.Millisecond), map[string]int{"dps": 1}, 1),
		ticket("dps-diana", now.Add(20*time.Millisecond), map[string]int{"dps": 1}, 1),
		ticket("dps-dave", now.Add(30*time.Millisecond), map[string]int{"dps": 1}, 1),
		ticket("healer-helen", now.Add(40*time.Millisecond), map[string]int{"support": 1}, 1),
		ticket("tank-tina", now.Add(50*time.Millisecond), map[string]int{"tank": 1}, 1),
		ticket("dps-dora", now.Add(60*time.Millisecond), map[string]int{"dps": 1}, 1),
		ticket("dps-derek", now.Add(70*time.Millisecond), map[string]int{"dps": 1}, 1),
		ticket("dps-daisy", now.Add(80*time.Millisecond), map[string]int{"dps": 1}, 1),
		ticket("healer-hank", now.Add(90*time.Millisecond), map[string]int{"support": 1}, 1),
	)

	profile := &mockProfile{
		teamCount:   2,
		teamSize:    5,
		teamMinSize: 5,
		teamNames:   []string{"team_1", "team_2"},
		composition: map[string]int{"tank": 1, "dps": 3, "support": 1},
	}

	// Expected exact assignments - greedy fill: team_1 first, then team_2
	// Each team needs: 1 tank, 3 dps, 1 support
	// Greedy assigns first valid team, so:
	// - tank-tom → team_1 (first tank)
	// - dps-dan → team_1 (first dps)
	// - dps-diana → team_1 (second dps)
	// - dps-dave → team_1 (third dps, team_1 dps full)
	// - healer-helen → team_1 (first support, team_1 complete!)
	// - tank-tina → team_2 (first tank for team_2)
	// - dps-dora → team_2 (first dps)
	// - dps-derek → team_2 (second dps)
	// - dps-daisy → team_2 (third dps)
	// - healer-hank → team_2 (support, team_2 complete!)
	expectedAssignments := []struct {
		ticketID  string
		teamIndex int
	}{
		{"tank-tom", 0},
		{"dps-dan", 0},
		{"dps-diana", 0},
		{"dps-dave", 0},
		{"healer-helen", 0},
		{"tank-tina", 1},
		{"dps-dora", 1},
		{"dps-derek", 1},
		{"dps-daisy", 1},
		{"healer-hank", 1},
	}

	// Run multiple times to verify determinism
	for i := 0; i < 10; i++ {
		input := NewInput(candidates, profile, now.Add(100*time.Millisecond))
		got := Run(input)

		if !got.Success {
			t.Fatalf("Run %d: expected Success = true", i)
		}

		if len(got.Assignments) != 10 {
			t.Fatalf("Run %d: expected 10 assignments, got %d", i, len(got.Assignments))
		}

		// Verify exact match with expected
		for j, a := range got.Assignments {
			if a.Ticket.GetID() != expectedAssignments[j].ticketID {
				t.Errorf("Run %d: assignment[%d] ticket = %s, expected %s",
					i, j, a.Ticket.GetID(), expectedAssignments[j].ticketID)
			}
			if a.TeamIndex != expectedAssignments[j].teamIndex {
				t.Errorf("Run %d: assignment[%d] team = %d, expected %d",
					i, j, a.TeamIndex, expectedAssignments[j].teamIndex)
			}
		}
	}
}

func TestDeterministic1v1(t *testing.T) {
	t.Parallel()

	now := time.Now()

	candidates := tickets(
		ticket("pro-john", now.Add(0*time.Millisecond), map[string]int{"default": 1}, 1),
		ticket("elite-eva", now.Add(10*time.Millisecond), map[string]int{"default": 1}, 1),
	)

	profile := &mockProfile{
		teamCount:   2,
		teamSize:    1,
		teamMinSize: 1,
		teamNames:   []string{"team_1", "team_2"},
	}

	// Expected exact assignments - greedy fill: team_1 first, then team_2
	// pro-john created first → team_1
	// elite-eva created second → team_2
	expectedAssignments := []struct {
		ticketID  string
		teamIndex int
	}{
		{"pro-john", 0},
		{"elite-eva", 1},
	}

	// Run multiple times to verify determinism
	for i := 0; i < 10; i++ {
		input := NewInput(candidates, profile, now.Add(100*time.Millisecond))
		got := Run(input)

		if !got.Success {
			t.Fatalf("Run %d: expected Success = true", i)
		}

		if len(got.Assignments) != 2 {
			t.Fatalf("Run %d: expected 2 assignments, got %d", i, len(got.Assignments))
		}

		for j, a := range got.Assignments {
			if a.Ticket.GetID() != expectedAssignments[j].ticketID {
				t.Errorf("Run %d: assignment[%d] ticket = %s, expected %s",
					i, j, a.Ticket.GetID(), expectedAssignments[j].ticketID)
			}
			if a.TeamIndex != expectedAssignments[j].teamIndex {
				t.Errorf("Run %d: assignment[%d] team = %d, expected %d",
					i, j, a.TeamIndex, expectedAssignments[j].teamIndex)
			}
		}
	}
}

func TestDeterministic2v2(t *testing.T) {
	t.Parallel()

	now := time.Now()

	candidates := tickets(
		ticket("alpha-andy", now.Add(0*time.Millisecond), map[string]int{"default": 1}, 1),
		ticket("beta-bob", now.Add(10*time.Millisecond), map[string]int{"default": 1}, 1),
		ticket("gamma-gary", now.Add(20*time.Millisecond), map[string]int{"default": 1}, 1),
		ticket("delta-dan", now.Add(30*time.Millisecond), map[string]int{"default": 1}, 1),
	)

	profile := &mockProfile{
		teamCount:   2,
		teamSize:    2,
		teamMinSize: 2,
		teamNames:   []string{"team_1", "team_2"},
	}

	// Expected exact assignments
	expectedAssignments := []struct {
		ticketID  string
		teamIndex int
	}{
		{"alpha-andy", 0},
		{"beta-bob", 0},
		{"gamma-gary", 1},
		{"delta-dan", 1},
	}

	// Run multiple times to verify determinism
	for i := 0; i < 10; i++ {
		input := NewInput(candidates, profile, now.Add(100*time.Millisecond))
		got := Run(input)

		if !got.Success {
			t.Fatalf("Run %d: expected Success = true", i)
		}

		if len(got.Assignments) != 4 {
			t.Fatalf("Run %d: expected 4 assignments, got %d", i, len(got.Assignments))
		}

		for j, a := range got.Assignments {
			if a.Ticket.GetID() != expectedAssignments[j].ticketID {
				t.Errorf("Run %d: assignment[%d] ticket = %s, expected %s",
					i, j, a.Ticket.GetID(), expectedAssignments[j].ticketID)
			}
			if a.TeamIndex != expectedAssignments[j].teamIndex {
				t.Errorf("Run %d: assignment[%d] team = %d, expected %d",
					i, j, a.TeamIndex, expectedAssignments[j].teamIndex)
			}
		}
	}
}

func TestDeterministic4TeamSquads(t *testing.T) {
	t.Parallel()

	now := time.Now()

	// 8 players for 4 teams of 2 (min_size=2, max_size=3)
	candidates := tickets(
		ticket("alpha1", now.Add(0*time.Millisecond), map[string]int{"default": 1}, 1),
		ticket("alpha2", now.Add(10*time.Millisecond), map[string]int{"default": 1}, 1),
		ticket("bravo1", now.Add(20*time.Millisecond), map[string]int{"default": 1}, 1),
		ticket("bravo2", now.Add(30*time.Millisecond), map[string]int{"default": 1}, 1),
		ticket("charlie1", now.Add(40*time.Millisecond), map[string]int{"default": 1}, 1),
		ticket("charlie2", now.Add(50*time.Millisecond), map[string]int{"default": 1}, 1),
		ticket("delta1", now.Add(60*time.Millisecond), map[string]int{"default": 1}, 1),
		ticket("delta2", now.Add(70*time.Millisecond), map[string]int{"default": 1}, 1),
	)

	profile := &asymmetricProfile{
		teams: []teamDef{
			{name: "team_1", size: 3, minSize: 2},
			{name: "team_2", size: 3, minSize: 2},
			{name: "team_3", size: 3, minSize: 2},
			{name: "team_4", size: 3, minSize: 2},
		},
	}

	// Run to verify it works
	input := NewInput(candidates, profile, now.Add(100*time.Millisecond))
	got := Run(input)

	if !got.Success {
		t.Fatalf("expected Success = true")
	}

	if len(got.Assignments) != 8 {
		t.Fatalf("expected 8 assignments, got %d", len(got.Assignments))
	}

	// Count players per team
	teamCounts := make(map[int]int)
	for _, a := range got.Assignments {
		teamCounts[a.TeamIndex]++
	}

	t.Logf("Team distribution: %v", teamCounts)

	// Each team should have exactly 2 players (8 players / 4 teams)
	for teamIdx := 0; teamIdx < 4; teamIdx++ {
		if teamCounts[teamIdx] != 2 {
			t.Errorf("team_%d has %d players, expected 2", teamIdx+1, teamCounts[teamIdx])
		}
	}

	// Log assignments for debugging
	for _, a := range got.Assignments {
		t.Logf("  %s -> team_%d", a.Ticket.GetID(), a.TeamIndex+1)
	}
}

// TestBackfillCase1_SinglePlayer tests backfill case 1 from the demo.
// Scenario: 2v2 match is running, 1 player leaves from team_1.
// A replacement ticket is created, backfill should match it.
func TestBackfillCase1_SinglePlayer(t *testing.T) {
	t.Parallel()

	now := time.Now()

	// Replacement player ticket (single player for 1 slot)
	candidates := tickets(
		ticket("alpha-andy-replacement", now, map[string]int{"default": 1}, 1),
	)

	// Backfill needs 1 slot in default pool
	slots := []SlotNeeded{{PoolName: "default", Count: 1}}

	input := NewBackfillInput(candidates, slots, now)
	got := Run(input)

	if !got.Success {
		t.Fatal("expected Success = true")
	}

	if len(got.Assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(got.Assignments))
	}

	if got.Assignments[0].Ticket.GetID() != "alpha-andy-replacement" {
		t.Errorf("expected ticket alpha-andy-replacement, got %s", got.Assignments[0].Ticket.GetID())
	}
}

// TestBackfillCase2_Party tests backfill case 2 from the demo.
// Scenario: 2v2 match is running, entire team_1 (2 players) leaves.
// Two replacement tickets are created, backfill should match both.
func TestBackfillCase2_Party(t *testing.T) {
	t.Parallel()

	now := time.Now()

	// Replacement party tickets (2 players for 2 slots)
	candidates := tickets(
		ticket("alpha-andy-replacement", now.Add(0*time.Millisecond), map[string]int{"default": 1}, 1),
		ticket("beta-bob-replacement", now.Add(10*time.Millisecond), map[string]int{"default": 1}, 1),
	)

	// Backfill needs 2 slots in default pool
	slots := []SlotNeeded{{PoolName: "default", Count: 2}}

	input := NewBackfillInput(candidates, slots, now)
	got := Run(input)

	if !got.Success {
		t.Fatal("expected Success = true")
	}

	if len(got.Assignments) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(got.Assignments))
	}

	// Verify oldest tickets are picked
	gotIDs := make(map[string]bool)
	for _, a := range got.Assignments {
		gotIDs[a.Ticket.GetID()] = true
	}

	if !gotIDs["alpha-andy-replacement"] || !gotIDs["beta-bob-replacement"] {
		t.Errorf("expected both replacement tickets, got %v", gotIDs)
	}
}

// TestBackfillCase3_RoleBased tests backfill case 3 from the demo.
// Scenario: 5v5 match is running, tank + dps leave from team_1.
// Two role-specific replacement tickets are created (BfTank, BfDps).
func TestBackfillCase3_RoleBased(t *testing.T) {
	t.Parallel()

	now := time.Now()

	// Role-specific replacement tickets
	candidates := tickets(
		ticket("bf-tank", now.Add(0*time.Millisecond), map[string]int{"tank": 1}, 1),
		ticket("bf-dps", now.Add(10*time.Millisecond), map[string]int{"dps": 1}, 1),
		// Extra candidates that shouldn't be matched
		ticket("bf-support", now.Add(20*time.Millisecond), map[string]int{"support": 1}, 1),
	)

	// Backfill needs 1 tank + 1 dps
	slots := []SlotNeeded{
		{PoolName: "tank", Count: 1},
		{PoolName: "dps", Count: 1},
	}

	input := NewBackfillInput(candidates, slots, now)
	got := Run(input)

	if !got.Success {
		t.Fatal("expected Success = true")
	}

	if len(got.Assignments) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(got.Assignments))
	}

	// Verify correct roles are matched
	gotIDs := make(map[string]bool)
	for _, a := range got.Assignments {
		gotIDs[a.Ticket.GetID()] = true
	}

	if !gotIDs["bf-tank"] {
		t.Error("expected bf-tank to be matched")
	}
	if !gotIDs["bf-dps"] {
		t.Error("expected bf-dps to be matched")
	}
	if gotIDs["bf-support"] {
		t.Error("bf-support should not be matched (not needed)")
	}
}

// TestBackfillCase4_MidGameDisconnect tests backfill case 4 from the demo.
// Scenario: 5v5 match is running, 1 support disconnects mid-game from team_1.
// A support replacement ticket is created.
func TestBackfillCase4_MidGameDisconnect(t *testing.T) {
	t.Parallel()

	now := time.Now()

	// Support replacement ticket
	candidates := tickets(
		ticket("bf-support", now, map[string]int{"support": 1}, 1),
	)

	// Backfill needs 1 support
	slots := []SlotNeeded{{PoolName: "support", Count: 1}}

	input := NewBackfillInput(candidates, slots, now)
	got := Run(input)

	if !got.Success {
		t.Fatal("expected Success = true")
	}

	if len(got.Assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(got.Assignments))
	}

	if got.Assignments[0].Ticket.GetID() != "bf-support" {
		t.Errorf("expected bf-support, got %s", got.Assignments[0].Ticket.GetID())
	}
}

// TestBackfillCase4_WrongRole tests that backfill fails when wrong role is available.
// Scenario: Support slot needs filling, but only DPS players are in queue.
func TestBackfillCase4_WrongRole(t *testing.T) {
	t.Parallel()

	now := time.Now()

	// Only DPS candidates available
	candidates := tickets(
		ticket("dps1", now, map[string]int{"dps": 1}, 1),
		ticket("dps2", now, map[string]int{"dps": 1}, 1),
	)

	// Backfill needs 1 support
	slots := []SlotNeeded{{PoolName: "support", Count: 1}}

	input := NewBackfillInput(candidates, slots, now)
	got := Run(input)

	if got.Success {
		t.Fatal("expected Success = false (no support available)")
	}

	if len(got.Assignments) != 0 {
		t.Errorf("expected 0 assignments, got %d", len(got.Assignments))
	}
}

// TestBackfillMultipleSlotsSameRole tests backfill with multiple slots of the same role.
// Scenario: 2 DPS players leave, need 2 DPS replacements.
func TestBackfillMultipleSlotsSameRole(t *testing.T) {
	t.Parallel()

	now := time.Now()

	candidates := tickets(
		ticket("dps1", now.Add(0*time.Millisecond), map[string]int{"dps": 1}, 1),
		ticket("dps2", now.Add(10*time.Millisecond), map[string]int{"dps": 1}, 1),
		ticket("dps3", now.Add(20*time.Millisecond), map[string]int{"dps": 1}, 1), // extra
	)

	// Backfill needs 2 DPS
	slots := []SlotNeeded{{PoolName: "dps", Count: 2}}

	input := NewBackfillInput(candidates, slots, now)
	got := Run(input)

	if !got.Success {
		t.Fatal("expected Success = true")
	}

	if len(got.Assignments) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(got.Assignments))
	}

	// Verify oldest 2 are picked (dps1, dps2)
	gotIDs := make(map[string]bool)
	for _, a := range got.Assignments {
		gotIDs[a.Ticket.GetID()] = true
	}

	if !gotIDs["dps1"] || !gotIDs["dps2"] {
		t.Errorf("expected dps1 and dps2 (oldest), got %v", gotIDs)
	}
	if gotIDs["dps3"] {
		t.Error("dps3 should not be matched (too new)")
	}
}

// TestBackfillPrioritizesOldestTickets tests that backfill picks oldest tickets first.
func TestBackfillPrioritizesOldestTickets(t *testing.T) {
	t.Parallel()

	now := time.Now()

	candidates := tickets(
		ticket("new-player", now.Add(-1*time.Minute), map[string]int{"default": 1}, 1),
		ticket("old-player", now.Add(-10*time.Minute), map[string]int{"default": 1}, 1),
		ticket("oldest-player", now.Add(-20*time.Minute), map[string]int{"default": 1}, 1),
	)

	// Only need 1 slot
	slots := []SlotNeeded{{PoolName: "default", Count: 1}}

	input := NewBackfillInput(candidates, slots, now)
	got := Run(input)

	if !got.Success {
		t.Fatal("expected Success = true")
	}

	if len(got.Assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(got.Assignments))
	}

	// Should pick the oldest player
	if got.Assignments[0].Ticket.GetID() != "oldest-player" {
		t.Errorf("expected oldest-player, got %s", got.Assignments[0].Ticket.GetID())
	}
}

// TestDeterministic4TeamSquadsWithComposition tests 4-team matching WITH pool composition.
// This is the real-world scenario where profile_loader generates composition from pools.
// Each team needs 2 players from "default" pool (composition: {"default": 2}).
func TestDeterministic4TeamSquadsWithComposition(t *testing.T) {
	t.Parallel()

	now := time.Now()

	// 8 players for 4 teams of 2
	candidates := tickets(
		ticket("alpha1", now.Add(0*time.Millisecond), map[string]int{"default": 1}, 1),
		ticket("alpha2", now.Add(10*time.Millisecond), map[string]int{"default": 1}, 1),
		ticket("bravo1", now.Add(20*time.Millisecond), map[string]int{"default": 1}, 1),
		ticket("bravo2", now.Add(30*time.Millisecond), map[string]int{"default": 1}, 1),
		ticket("charlie1", now.Add(40*time.Millisecond), map[string]int{"default": 1}, 1),
		ticket("charlie2", now.Add(50*time.Millisecond), map[string]int{"default": 1}, 1),
		ticket("delta1", now.Add(60*time.Millisecond), map[string]int{"default": 1}, 1),
		ticket("delta2", now.Add(70*time.Millisecond), map[string]int{"default": 1}, 1),
	)

	// This profile has composition - each team needs exactly 2 "default" pool players
	// This simulates what profile_loader.go generates: teamPools = ["default", "default"]
	profile := &asymmetricProfile{
		teams: []teamDef{
			{name: "team_1", size: 3, minSize: 2, composition: map[string]int{"default": 2}},
			{name: "team_2", size: 3, minSize: 2, composition: map[string]int{"default": 2}},
			{name: "team_3", size: 3, minSize: 2, composition: map[string]int{"default": 2}},
			{name: "team_4", size: 3, minSize: 2, composition: map[string]int{"default": 2}},
		},
	}

	// Run to verify it works
	input := NewInput(candidates, profile, now.Add(100*time.Millisecond))
	got := Run(input)

	if !got.Success {
		t.Fatalf("expected Success = true, got false (this would fail if composition requires 2 but only 1 slot)")
	}

	if len(got.Assignments) != 8 {
		t.Fatalf("expected 8 assignments, got %d", len(got.Assignments))
	}

	// Count players per team
	teamCounts := make(map[int]int)
	for _, a := range got.Assignments {
		teamCounts[a.TeamIndex]++
	}

	t.Logf("Team distribution: %v", teamCounts)

	// Each team should have exactly 2 players
	for teamIdx := 0; teamIdx < 4; teamIdx++ {
		if teamCounts[teamIdx] != 2 {
			t.Errorf("team_%d has %d players, expected 2", teamIdx+1, teamCounts[teamIdx])
		}
	}

	// Run multiple times to verify determinism
	for i := 0; i < 10; i++ {
		input := NewInput(candidates, profile, now.Add(100*time.Millisecond))
		result := Run(input)

		if !result.Success {
			t.Fatalf("Run %d: expected Success = true", i)
		}

		if len(result.Assignments) != 8 {
			t.Fatalf("Run %d: expected 8 assignments, got %d", i, len(result.Assignments))
		}

		// Verify same assignments each run
		for j, a := range result.Assignments {
			if a.Ticket.GetID() != got.Assignments[j].Ticket.GetID() {
				t.Errorf("Run %d: assignment[%d] ticket = %s, expected %s (non-deterministic!)",
					i, j, a.Ticket.GetID(), got.Assignments[j].Ticket.GetID())
			}
			if a.TeamIndex != got.Assignments[j].TeamIndex {
				t.Errorf("Run %d: assignment[%d] team = %d, expected %d (non-deterministic!)",
					i, j, a.TeamIndex, got.Assignments[j].TeamIndex)
			}
		}
	}

	// Log the deterministic assignments
	t.Log("Deterministic assignments:")
	for _, a := range got.Assignments {
		t.Logf("  %s -> team_%d", a.Ticket.GetID(), a.TeamIndex+1)
	}
}
