package store

import (
	"testing"
	"time"
)

func TestPartyStore_Create(t *testing.T) {
	store := NewPartyStore()
	now := time.Now()

	party, err := store.Create("leader-1", true, 5, now)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if party.ID == "" {
		t.Error("expected party ID to be set")
	}
	if party.LeaderID != "leader-1" {
		t.Errorf("expected leader ID to be leader-1, got %s", party.LeaderID)
	}
	if !party.IsOpen {
		t.Error("expected party to be open")
	}
	if party.MaxSize != 5 {
		t.Errorf("expected max size 5, got %d", party.MaxSize)
	}
	if len(party.Members) != 1 || party.Members[0] != "leader-1" {
		t.Errorf("expected members to contain leader, got %v", party.Members)
	}
}

func TestPartyStore_CreateDuplicate(t *testing.T) {
	store := NewPartyStore()
	now := time.Now()

	_, err := store.Create("leader-1", true, 5, now)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Try to create another party with same leader
	_, err = store.Create("leader-1", true, 5, now)
	if err == nil {
		t.Error("expected error when creating party with same leader")
	}
}

func TestPartyStore_GetByPlayer(t *testing.T) {
	store := NewPartyStore()
	now := time.Now()

	party, _ := store.Create("leader-1", true, 5, now)

	// Get by leader
	found, ok := store.GetByPlayer("leader-1")
	if !ok {
		t.Error("expected to find party by leader")
	}
	if found.ID != party.ID {
		t.Errorf("expected party ID %s, got %s", party.ID, found.ID)
	}

	// Try non-existent player
	_, ok = store.GetByPlayer("nonexistent")
	if ok {
		t.Error("expected not to find party for nonexistent player")
	}
}

func TestPartyStore_AddMember(t *testing.T) {
	store := NewPartyStore()
	now := time.Now()

	party, _ := store.Create("leader-1", true, 5, now)

	err := store.AddMember(party.ID, "player-2")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	party, _ = store.Get(party.ID)
	if len(party.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(party.Members))
	}

	// Verify player can be found by party
	found, ok := store.GetByPlayer("player-2")
	if !ok || found.ID != party.ID {
		t.Error("expected to find party by new member")
	}
}

func TestPartyStore_AddMemberToClosedParty(t *testing.T) {
	store := NewPartyStore()
	now := time.Now()

	party, _ := store.Create("leader-1", false, 5, now) // Not open

	err := store.AddMember(party.ID, "player-2")
	if err == nil {
		t.Error("expected error when joining closed party")
	}
}

func TestPartyStore_AddMemberToFullParty(t *testing.T) {
	store := NewPartyStore()
	now := time.Now()

	party, _ := store.Create("leader-1", true, 2, now) // Max 2
	_ = store.AddMember(party.ID, "player-2")

	err := store.AddMember(party.ID, "player-3")
	if err == nil {
		t.Error("expected error when joining full party")
	}
}

func TestPartyStore_RemoveMember(t *testing.T) {
	store := NewPartyStore()
	now := time.Now()

	party, _ := store.Create("leader-1", true, 5, now)
	_ = store.AddMember(party.ID, "player-2")

	disbanded, err := store.RemoveMember(party.ID, "player-2")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if disbanded {
		t.Error("expected party not to be disbanded")
	}

	party, _ = store.Get(party.ID)
	if len(party.Members) != 1 {
		t.Errorf("expected 1 member, got %d", len(party.Members))
	}

	// Player should no longer be found
	_, ok := store.GetByPlayer("player-2")
	if ok {
		t.Error("expected removed player not to be found")
	}
}

func TestPartyStore_RemoveLeader(t *testing.T) {
	store := NewPartyStore()
	now := time.Now()

	party, _ := store.Create("leader-1", true, 5, now)
	_ = store.AddMember(party.ID, "player-2")

	disbanded, err := store.RemoveMember(party.ID, "leader-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if disbanded {
		t.Error("expected party not to be disbanded")
	}

	party, _ = store.Get(party.ID)
	if party.LeaderID != "player-2" {
		t.Errorf("expected new leader to be player-2, got %s", party.LeaderID)
	}
}

func TestPartyStore_DisbandOnLastMember(t *testing.T) {
	store := NewPartyStore()
	now := time.Now()

	party, _ := store.Create("leader-1", true, 5, now)

	disbanded, err := store.RemoveMember(party.ID, "leader-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !disbanded {
		t.Error("expected party to be disbanded")
	}

	_, ok := store.Get(party.ID)
	if ok {
		t.Error("expected party to be deleted")
	}
}

func TestPartyStore_SetLeader(t *testing.T) {
	store := NewPartyStore()
	now := time.Now()

	party, _ := store.Create("leader-1", true, 5, now)
	_ = store.AddMember(party.ID, "player-2")

	err := store.SetLeader(party.ID, "player-2")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	party, _ = store.Get(party.ID)
	if party.LeaderID != "player-2" {
		t.Errorf("expected leader to be player-2, got %s", party.LeaderID)
	}
}

func TestPartyStore_SetLeaderNonMember(t *testing.T) {
	store := NewPartyStore()
	now := time.Now()

	party, _ := store.Create("leader-1", true, 5, now)

	err := store.SetLeader(party.ID, "nonexistent")
	if err == nil {
		t.Error("expected error when setting non-member as leader")
	}
}

func TestPartyStore_SetLobby(t *testing.T) {
	store := NewPartyStore()
	now := time.Now()

	party, _ := store.Create("leader-1", true, 5, now)

	err := store.SetLobby(party.ID, "lobby-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	party, _ = store.Get(party.ID)
	if party.LobbyID != "lobby-1" {
		t.Errorf("expected lobby ID to be lobby-1, got %s", party.LobbyID)
	}
	if !party.InLobby() {
		t.Error("expected party to be in lobby")
	}
}

func TestPartyStore_SetReady(t *testing.T) {
	store := NewPartyStore()
	now := time.Now()

	party, _ := store.Create("leader-1", true, 5, now)

	err := store.SetReady(party.ID, true)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	party, _ = store.Get(party.ID)
	if !party.IsReady {
		t.Error("expected party to be ready")
	}
}

func TestPartyStore_Delete(t *testing.T) {
	store := NewPartyStore()
	now := time.Now()

	party, _ := store.Create("leader-1", true, 5, now)
	_ = store.AddMember(party.ID, "player-2")

	ok := store.Delete(party.ID)
	if !ok {
		t.Error("expected delete to succeed")
	}

	_, found := store.Get(party.ID)
	if found {
		t.Error("expected party to be deleted")
	}

	// All members should be removed from index
	_, found = store.GetByPlayer("leader-1")
	if found {
		t.Error("expected leader to be removed from index")
	}
	_, found = store.GetByPlayer("player-2")
	if found {
		t.Error("expected member to be removed from index")
	}
}

func TestPartyStore_Count(t *testing.T) {
	store := NewPartyStore()
	now := time.Now()

	if store.Count() != 0 {
		t.Errorf("expected count 0, got %d", store.Count())
	}

	store.Create("leader-1", true, 5, now)
	if store.Count() != 1 {
		t.Errorf("expected count 1, got %d", store.Count())
	}

	store.Create("leader-2", true, 5, now)
	if store.Count() != 2 {
		t.Errorf("expected count 2, got %d", store.Count())
	}
}

func TestPartyStore_Clear(t *testing.T) {
	store := NewPartyStore()
	now := time.Now()

	store.Create("leader-1", true, 5, now)
	store.Create("leader-2", true, 5, now)

	store.Clear()

	if store.Count() != 0 {
		t.Errorf("expected count 0 after clear, got %d", store.Count())
	}
}

func TestPartyStore_RestoreAndCounter(t *testing.T) {
	store := NewPartyStore()
	now := time.Now()

	party, _ := store.Create("leader-1", true, 5, now)
	_ = store.AddMember(party.ID, "player-2")

	// Get all for snapshot
	parties := store.All()
	counter := store.GetCounter()

	// Clear and restore
	store.Clear()
	store.SetCounter(counter)
	for _, p := range parties {
		store.Restore(p)
	}

	// Verify restoration
	if store.Count() != 1 {
		t.Errorf("expected count 1, got %d", store.Count())
	}

	restored, ok := store.Get(party.ID)
	if !ok {
		t.Error("expected to find restored party")
	}
	if len(restored.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(restored.Members))
	}

	// Verify indexes restored
	_, ok = store.GetByPlayer("leader-1")
	if !ok {
		t.Error("expected leader index to be restored")
	}
	_, ok = store.GetByPlayer("player-2")
	if !ok {
		t.Error("expected member index to be restored")
	}
}
