package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/matchmaking/types"
)

func TestTicketStore_Create(t *testing.T) {
	store := NewTicketStore()
	now := time.Now()

	players := []types.PlayerInfo{
		{PlayerID: "player1"},
	}
	poolCounts := map[string]int{"default": 1}

	ticket, err := store.Create("party1", "1v1-ranked", true, players, now, time.Hour, poolCounts)

	require.NoError(t, err)
	assert.NotEmpty(t, ticket.ID)
	assert.Equal(t, "party1", ticket.PartyID)
	assert.Equal(t, "1v1-ranked", ticket.MatchProfileName)
	assert.True(t, ticket.AllowBackfill)
	assert.Equal(t, players, ticket.Players)
	assert.Equal(t, now, ticket.CreatedAt)
	assert.Equal(t, now.Add(time.Hour), ticket.ExpiresAt)
	assert.Equal(t, poolCounts, ticket.PoolCounts)
}

func TestTicketStore_Create_DuplicateParty(t *testing.T) {
	store := NewTicketStore()
	now := time.Now()

	players := []types.PlayerInfo{{PlayerID: "player1"}}
	poolCounts := map[string]int{"default": 1}

	// First ticket should succeed
	_, err := store.Create("party1", "1v1-ranked", true, players, now, time.Hour, poolCounts)
	require.NoError(t, err)

	// Second ticket with same party should fail
	_, err = store.Create("party1", "1v1-ranked", true, players, now, time.Hour, poolCounts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already has an active ticket")
}

func TestTicketStore_Get(t *testing.T) {
	store := NewTicketStore()
	now := time.Now()

	players := []types.PlayerInfo{{PlayerID: "player1"}}
	ticket, _ := store.Create("party1", "1v1-ranked", true, players, now, time.Hour, nil)

	// Get existing ticket
	found, ok := store.Get(ticket.ID)
	require.True(t, ok)
	assert.Equal(t, ticket.ID, found.ID)

	// Get non-existent ticket
	_, ok = store.Get("non-existent")
	assert.False(t, ok)
}

func TestTicketStore_GetByParty(t *testing.T) {
	store := NewTicketStore()
	now := time.Now()

	players := []types.PlayerInfo{{PlayerID: "player1"}}
	ticket, _ := store.Create("party1", "1v1-ranked", true, players, now, time.Hour, nil)

	// Get existing party's ticket
	found, ok := store.GetByParty("party1")
	require.True(t, ok)
	assert.Equal(t, ticket.ID, found.ID)

	// Get non-existent party
	_, ok = store.GetByParty("non-existent")
	assert.False(t, ok)
}

func TestTicketStore_Delete(t *testing.T) {
	store := NewTicketStore()
	now := time.Now()

	players := []types.PlayerInfo{{PlayerID: "player1"}}
	ticket, _ := store.Create("party1", "1v1-ranked", true, players, now, time.Hour, nil)

	// Delete should succeed
	ok := store.Delete(ticket.ID)
	assert.True(t, ok)

	// Should no longer be found
	_, found := store.Get(ticket.ID)
	assert.False(t, found)

	// Party should be free to create new ticket
	_, err := store.Create("party1", "1v1-ranked", true, players, now, time.Hour, nil)
	assert.NoError(t, err)

	// Delete non-existent should return false
	ok = store.Delete("non-existent")
	assert.False(t, ok)
}

func TestTicketStore_DeleteMultiple(t *testing.T) {
	store := NewTicketStore()
	now := time.Now()

	players := []types.PlayerInfo{{PlayerID: "player1"}}
	ticket1, _ := store.Create("party1", "1v1-ranked", true, players, now, time.Hour, nil)
	ticket2, _ := store.Create("party2", "1v1-ranked", true, players, now, time.Hour, nil)
	ticket3, _ := store.Create("party3", "1v1-ranked", true, players, now, time.Hour, nil)

	store.DeleteMultiple([]string{ticket1.ID, ticket2.ID})

	// ticket1 and ticket2 should be deleted
	_, ok := store.Get(ticket1.ID)
	assert.False(t, ok)
	_, ok = store.Get(ticket2.ID)
	assert.False(t, ok)

	// ticket3 should still exist
	_, ok = store.Get(ticket3.ID)
	assert.True(t, ok)
}

func TestTicketStore_GetByProfile(t *testing.T) {
	store := NewTicketStore()
	now := time.Now()

	players := []types.PlayerInfo{{PlayerID: "player1"}}

	// Create tickets for different profiles
	store.Create("party1", "1v1-ranked", true, players, now, time.Hour, nil)
	store.Create("party2", "1v1-ranked", true, players, now.Add(time.Second), time.Hour, nil)
	store.Create("party3", "2v2-competitive", true, players, now, time.Hour, nil)

	ranked := store.GetByProfile("1v1-ranked")
	assert.Len(t, ranked, 2)

	competitive := store.GetByProfile("2v2-competitive")
	assert.Len(t, competitive, 1)

	nonExistent := store.GetByProfile("non-existent")
	assert.Len(t, nonExistent, 0)
}

func TestTicketStore_GetByProfile_SortedByCreatedAt(t *testing.T) {
	store := NewTicketStore()
	now := time.Now()

	players := []types.PlayerInfo{{PlayerID: "player1"}}

	// Create tickets in non-chronological order
	store.Create("party2", "1v1-ranked", true, players, now.Add(2*time.Second), time.Hour, nil)
	store.Create("party1", "1v1-ranked", true, players, now, time.Hour, nil)
	store.Create("party3", "1v1-ranked", true, players, now.Add(time.Second), time.Hour, nil)

	tickets := store.GetByProfile("1v1-ranked")
	require.Len(t, tickets, 3)

	// Should be sorted oldest first
	assert.Equal(t, "party1", tickets[0].PartyID)
	assert.Equal(t, "party3", tickets[1].PartyID)
	assert.Equal(t, "party2", tickets[2].PartyID)
}

func TestTicketStore_GetBackfillEligible(t *testing.T) {
	store := NewTicketStore()
	now := time.Now()

	players := []types.PlayerInfo{{PlayerID: "player1"}}

	// Create tickets with different backfill settings
	store.Create("party1", "1v1-ranked", true, players, now, time.Hour, nil)
	store.Create("party2", "1v1-ranked", false, players, now, time.Hour, nil)
	store.Create("party3", "1v1-ranked", true, players, now, time.Hour, nil)

	eligible := store.GetBackfillEligible("1v1-ranked")
	assert.Len(t, eligible, 2)

	for _, ticket := range eligible {
		assert.True(t, ticket.AllowBackfill)
	}
}

func TestTicketStore_ExpireTickets(t *testing.T) {
	store := NewTicketStore()
	now := time.Now()

	players := []types.PlayerInfo{{PlayerID: "player1"}}

	// Create tickets with different expiration times
	store.Create("party1", "1v1-ranked", true, players, now.Add(-2*time.Hour), time.Hour, nil) // Expired
	store.Create("party2", "1v1-ranked", true, players, now.Add(-30*time.Minute), time.Hour, nil) // Not expired
	store.Create("party3", "1v1-ranked", true, players, now.Add(-90*time.Minute), time.Hour, nil) // Expired

	expired := store.ExpireTickets(now)
	assert.Equal(t, 2, expired)

	// Only party2 should remain
	assert.Equal(t, 1, store.Count())
	_, ok := store.GetByParty("party2")
	assert.True(t, ok)
}

func TestTicketStore_Count(t *testing.T) {
	store := NewTicketStore()
	now := time.Now()

	players := []types.PlayerInfo{{PlayerID: "player1"}}

	assert.Equal(t, 0, store.Count())

	store.Create("party1", "1v1-ranked", true, players, now, time.Hour, nil)
	assert.Equal(t, 1, store.Count())

	store.Create("party2", "1v1-ranked", true, players, now, time.Hour, nil)
	assert.Equal(t, 2, store.Count())
}

func TestTicketStore_CountByProfile(t *testing.T) {
	store := NewTicketStore()
	now := time.Now()

	players := []types.PlayerInfo{{PlayerID: "player1"}}

	store.Create("party1", "1v1-ranked", true, players, now, time.Hour, nil)
	store.Create("party2", "1v1-ranked", true, players, now, time.Hour, nil)
	store.Create("party3", "2v2-competitive", true, players, now, time.Hour, nil)

	assert.Equal(t, 2, store.CountByProfile("1v1-ranked"))
	assert.Equal(t, 1, store.CountByProfile("2v2-competitive"))
	assert.Equal(t, 0, store.CountByProfile("non-existent"))
}

func TestTicketStore_All(t *testing.T) {
	store := NewTicketStore()
	now := time.Now()

	players := []types.PlayerInfo{{PlayerID: "player1"}}

	store.Create("party1", "1v1-ranked", true, players, now, time.Hour, nil)
	store.Create("party2", "2v2-competitive", true, players, now, time.Hour, nil)

	all := store.All()
	assert.Len(t, all, 2)
}

func TestTicketStore_Clear(t *testing.T) {
	store := NewTicketStore()
	now := time.Now()

	players := []types.PlayerInfo{{PlayerID: "player1"}}

	store.Create("party1", "1v1-ranked", true, players, now, time.Hour, nil)
	store.Create("party2", "1v1-ranked", true, players, now, time.Hour, nil)

	store.Clear()

	assert.Equal(t, 0, store.Count())
	assert.Len(t, store.GetByProfile("1v1-ranked"), 0)
}

func TestTicketStore_Restore(t *testing.T) {
	store := NewTicketStore()
	now := time.Now()

	ticket := &types.Ticket{
		ID:               "restored-id",
		PartyID:          "party1",
		MatchProfileName: "1v1-ranked",
		AllowBackfill:    true,
		Players: []types.PlayerInfo{
			{PlayerID: "player1"},
		},
		CreatedAt: now,
		ExpiresAt: now.Add(time.Hour),
	}

	store.Restore(ticket)

	// Should be findable by ID
	found, ok := store.Get("restored-id")
	require.True(t, ok)
	assert.Equal(t, ticket, found)

	// Should be findable by party
	_, ok = store.GetByParty("party1")
	assert.True(t, ok)

	// Should be in profile index
	assert.Len(t, store.GetByProfile("1v1-ranked"), 1)

	// Should be in backfill index
	assert.Len(t, store.GetBackfillEligible("1v1-ranked"), 1)
}

func TestTicketStore_Counter(t *testing.T) {
	store := NewTicketStore()

	assert.Equal(t, uint64(0), store.GetCounter())

	store.SetCounter(100)
	assert.Equal(t, uint64(100), store.GetCounter())
}
