package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/matchmaking/types"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
)

func TestBackfillStore_Create(t *testing.T) {
	store := NewBackfillStore()
	now := time.Now()

	slotsNeeded := []types.SlotNeeded{
		{PoolName: "dps", Count: 2},
	}
	lobbyAddr := &microv1.ServiceAddress{
		Region:       "local",
		Organization: "demo",
		Project:      "test",
		ServiceId:    "lobby-1",
	}

	req := store.Create("match1", "5v5-roles", "team_1", slotsNeeded, lobbyAddr, now, time.Hour)

	assert.NotEmpty(t, req.ID)
	assert.Equal(t, "match1", req.MatchID)
	assert.Equal(t, "5v5-roles", req.MatchProfileName)
	assert.Equal(t, "team_1", req.TeamName)
	assert.Equal(t, slotsNeeded, req.SlotsNeeded)
	assert.Equal(t, lobbyAddr, req.LobbyAddress)
	assert.Equal(t, now, req.CreatedAt)
	assert.Equal(t, now.Add(time.Hour), req.ExpiresAt)
}

func TestBackfillStore_Get(t *testing.T) {
	store := NewBackfillStore()
	now := time.Now()

	slotsNeeded := []types.SlotNeeded{{PoolName: "dps", Count: 1}}
	req := store.Create("match1", "5v5-roles", "team_1", slotsNeeded, nil, now, time.Hour)

	// Get existing request
	found, ok := store.Get(req.ID)
	require.True(t, ok)
	assert.Equal(t, req.ID, found.ID)

	// Get non-existent request
	_, ok = store.Get("non-existent")
	assert.False(t, ok)
}

func TestBackfillStore_GetByMatch(t *testing.T) {
	store := NewBackfillStore()
	now := time.Now()

	slotsNeeded := []types.SlotNeeded{{PoolName: "dps", Count: 1}}

	// Create multiple backfill requests for same match
	store.Create("match1", "5v5-roles", "team_1", slotsNeeded, nil, now, time.Hour)
	store.Create("match1", "5v5-roles", "team_2", slotsNeeded, nil, now, time.Hour)
	store.Create("match2", "5v5-roles", "team_1", slotsNeeded, nil, now, time.Hour)

	// Get requests for match1
	match1Reqs := store.GetByMatch("match1")
	assert.Len(t, match1Reqs, 2)

	// Get requests for match2
	match2Reqs := store.GetByMatch("match2")
	assert.Len(t, match2Reqs, 1)

	// Get requests for non-existent match
	nonExistent := store.GetByMatch("non-existent")
	assert.Len(t, nonExistent, 0)
}

func TestBackfillStore_Delete(t *testing.T) {
	store := NewBackfillStore()
	now := time.Now()

	slotsNeeded := []types.SlotNeeded{{PoolName: "dps", Count: 1}}
	req := store.Create("match1", "5v5-roles", "team_1", slotsNeeded, nil, now, time.Hour)

	// Delete should succeed
	ok := store.Delete(req.ID)
	assert.True(t, ok)

	// Should no longer be found
	_, found := store.Get(req.ID)
	assert.False(t, found)

	// Delete non-existent should return false
	ok = store.Delete("non-existent")
	assert.False(t, ok)
}

func TestBackfillStore_Delete_RemovesFromMatchIndex(t *testing.T) {
	store := NewBackfillStore()
	now := time.Now()

	slotsNeeded := []types.SlotNeeded{{PoolName: "dps", Count: 1}}
	req1 := store.Create("match1", "5v5-roles", "team_1", slotsNeeded, nil, now, time.Hour)
	req2 := store.Create("match1", "5v5-roles", "team_2", slotsNeeded, nil, now, time.Hour)

	// Delete first request
	store.Delete(req1.ID)

	// Match should still have one request
	reqs := store.GetByMatch("match1")
	require.Len(t, reqs, 1)
	assert.Equal(t, req2.ID, reqs[0].ID)

	// Delete second request
	store.Delete(req2.ID)

	// Match should have no requests
	reqs = store.GetByMatch("match1")
	assert.Len(t, reqs, 0)
}

func TestBackfillStore_All(t *testing.T) {
	store := NewBackfillStore()
	now := time.Now()

	slotsNeeded := []types.SlotNeeded{{PoolName: "dps", Count: 1}}

	store.Create("match1", "5v5-roles", "team_1", slotsNeeded, nil, now, time.Hour)
	store.Create("match2", "5v5-roles", "team_1", slotsNeeded, nil, now, time.Hour)

	all := store.All()
	assert.Len(t, all, 2)
}

func TestBackfillStore_ExpireRequests(t *testing.T) {
	store := NewBackfillStore()
	now := time.Now()

	slotsNeeded := []types.SlotNeeded{{PoolName: "dps", Count: 1}}

	// Create requests with different expiration times
	store.Create("match1", "5v5-roles", "team_1", slotsNeeded, nil, now.Add(-2*time.Hour), time.Hour) // Expired
	store.Create("match2", "5v5-roles", "team_1", slotsNeeded, nil, now.Add(-30*time.Minute), time.Hour) // Not expired
	store.Create("match3", "5v5-roles", "team_1", slotsNeeded, nil, now.Add(-90*time.Minute), time.Hour) // Expired

	expired := store.ExpireRequests(now)
	assert.Equal(t, 2, expired)

	// Only match2 should remain
	assert.Equal(t, 1, store.Count())
}

func TestBackfillStore_Count(t *testing.T) {
	store := NewBackfillStore()
	now := time.Now()

	slotsNeeded := []types.SlotNeeded{{PoolName: "dps", Count: 1}}

	assert.Equal(t, 0, store.Count())

	store.Create("match1", "5v5-roles", "team_1", slotsNeeded, nil, now, time.Hour)
	assert.Equal(t, 1, store.Count())

	store.Create("match2", "5v5-roles", "team_1", slotsNeeded, nil, now, time.Hour)
	assert.Equal(t, 2, store.Count())
}

func TestBackfillStore_Clear(t *testing.T) {
	store := NewBackfillStore()
	now := time.Now()

	slotsNeeded := []types.SlotNeeded{{PoolName: "dps", Count: 1}}

	store.Create("match1", "5v5-roles", "team_1", slotsNeeded, nil, now, time.Hour)
	store.Create("match2", "5v5-roles", "team_1", slotsNeeded, nil, now, time.Hour)

	store.Clear()

	assert.Equal(t, 0, store.Count())
	assert.Len(t, store.All(), 0)
}

func TestBackfillStore_Restore(t *testing.T) {
	store := NewBackfillStore()
	now := time.Now()

	req := &types.BackfillRequest{
		ID:               "restored-id",
		MatchID:          "match1",
		MatchProfileName: "5v5-roles",
		TeamName:         "team_1",
		SlotsNeeded: []types.SlotNeeded{
			{PoolName: "dps", Count: 2},
		},
		CreatedAt: now,
		ExpiresAt: now.Add(time.Hour),
	}

	store.Restore(req)

	// Should be findable by ID
	found, ok := store.Get("restored-id")
	require.True(t, ok)
	assert.Equal(t, req, found)

	// Should be findable by match
	reqs := store.GetByMatch("match1")
	require.Len(t, reqs, 1)
	assert.Equal(t, req.ID, reqs[0].ID)
}

func TestBackfillStore_Counter(t *testing.T) {
	store := NewBackfillStore()

	assert.Equal(t, uint64(0), store.GetCounter())

	store.SetCounter(100)
	assert.Equal(t, uint64(100), store.GetCounter())
}
