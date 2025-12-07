package store

import (
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/lobby/types"
)

func TestLobbyStore_CreateFromMatch(t *testing.T) {
	store := NewLobbyStore()
	now := time.Now()

	teams := []types.LobbyTeam{
		{Name: "Red", PartyIDs: []string{"party-1", "party-2"}},
		{Name: "Blue", PartyIDs: []string{"party-3", "party-4"}},
	}

	lobby, err := store.CreateFromMatch("match-1", "5v5", teams, nil, nil, nil, now)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if lobby.MatchID != "match-1" {
		t.Errorf("expected match ID match-1, got %s", lobby.MatchID)
	}
	if lobby.MatchProfileName != "5v5" {
		t.Errorf("expected profile name 5v5, got %s", lobby.MatchProfileName)
	}
	if len(lobby.Teams) != 2 {
		t.Errorf("expected 2 teams, got %d", len(lobby.Teams))
	}
	if len(lobby.Parties) != 4 {
		t.Errorf("expected 4 parties, got %d", len(lobby.Parties))
	}
}

func TestLobbyStore_CreateFromMatchDuplicate(t *testing.T) {
	store := NewLobbyStore()
	now := time.Now()

	teams := []types.LobbyTeam{
		{Name: "Team1", PartyIDs: []string{"party-1"}},
	}
	_, err := store.CreateFromMatch("match-1", "1v1", teams, nil, nil, nil, now)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Try to create with same match ID
	_, err = store.CreateFromMatch("match-1", "1v1", teams, nil, nil, nil, now)
	if err == nil {
		t.Error("expected error when match already has a lobby")
	}
}

func TestLobbyStore_Get(t *testing.T) {
	store := NewLobbyStore()
	now := time.Now()

	teams := []types.LobbyTeam{
		{Name: "Team1", PartyIDs: []string{"party-1"}},
	}
	lobby, _ := store.CreateFromMatch("match-1", "1v1", teams, nil, nil, nil, now)

	found, ok := store.Get("match-1")
	if !ok {
		t.Error("expected to find lobby by match ID")
	}
	if found.MatchID != lobby.MatchID {
		t.Errorf("expected match ID %s, got %s", lobby.MatchID, found.MatchID)
	}

	_, ok = store.Get("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent match")
	}
}

func TestLobbyStore_GetByParty(t *testing.T) {
	store := NewLobbyStore()
	now := time.Now()

	teams := []types.LobbyTeam{
		{Name: "Team1", PartyIDs: []string{"party-1"}},
	}
	lobby, _ := store.CreateFromMatch("match-1", "1v1", teams, nil, nil, nil, now)

	found, ok := store.GetByParty("party-1")
	if !ok {
		t.Error("expected to find lobby by party ID")
	}
	if found.MatchID != lobby.MatchID {
		t.Errorf("expected match ID %s, got %s", lobby.MatchID, found.MatchID)
	}
}

func TestLobbyStore_AddParty(t *testing.T) {
	store := NewLobbyStore()
	now := time.Now()

	teams := []types.LobbyTeam{
		{Name: "Team1", PartyIDs: []string{"party-1"}},
	}
	lobby, _ := store.CreateFromMatch("match-1", "1v1", teams, nil, nil, nil, now)

	err := store.AddParty(lobby.MatchID, "party-2")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	lobby, _ = store.Get(lobby.MatchID)
	if len(lobby.Parties) != 2 {
		t.Errorf("expected 2 parties, got %d", len(lobby.Parties))
	}

	// Verify index
	found, ok := store.GetByParty("party-2")
	if !ok || found.MatchID != lobby.MatchID {
		t.Error("expected party-2 to be indexed to lobby")
	}
}

func TestLobbyStore_AddPartyDuplicate(t *testing.T) {
	store := NewLobbyStore()
	now := time.Now()

	teams1 := []types.LobbyTeam{{Name: "Team1", PartyIDs: []string{"party-1"}}}
	teams2 := []types.LobbyTeam{{Name: "Team1", PartyIDs: []string{"party-2"}}}

	lobby1, _ := store.CreateFromMatch("match-1", "1v1", teams1, nil, nil, nil, now)
	store.CreateFromMatch("match-2", "1v1", teams2, nil, nil, nil, now)

	// Try to add party-2 to lobby1 (but party-2 already has lobby2)
	err := store.AddParty(lobby1.MatchID, "party-2")
	if err == nil {
		t.Error("expected error when party already in another lobby")
	}
}

func TestLobbyStore_RemoveParty(t *testing.T) {
	store := NewLobbyStore()
	now := time.Now()

	teams := []types.LobbyTeam{
		{Name: "Team1", PartyIDs: []string{"party-1", "party-2"}},
	}
	lobby, _ := store.CreateFromMatch("match-1", "1v1", teams, nil, nil, nil, now)

	closed, err := store.RemoveParty(lobby.MatchID, "party-2")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if closed {
		t.Error("expected lobby not to be closed")
	}

	lobby, _ = store.Get(lobby.MatchID)
	if len(lobby.Parties) != 1 {
		t.Errorf("expected 1 party, got %d", len(lobby.Parties))
	}

	// Verify party removed from index
	_, ok := store.GetByParty("party-2")
	if ok {
		t.Error("expected party-2 to be removed from index")
	}
}

func TestLobbyStore_RemoveHost(t *testing.T) {
	store := NewLobbyStore()
	now := time.Now()

	teams := []types.LobbyTeam{
		{Name: "Team1", PartyIDs: []string{"party-1", "party-2"}},
	}
	lobby, _ := store.CreateFromMatch("match-1", "1v1", teams, nil, nil, nil, now)

	closed, err := store.RemoveParty(lobby.MatchID, "party-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if closed {
		t.Error("expected lobby not to be closed")
	}

	lobby, _ = store.Get(lobby.MatchID)
	if lobby.HostPartyID != "party-2" {
		t.Errorf("expected new host party-2, got %s", lobby.HostPartyID)
	}
}

func TestLobbyStore_CloseOnLastParty(t *testing.T) {
	store := NewLobbyStore()
	now := time.Now()

	teams := []types.LobbyTeam{
		{Name: "Team1", PartyIDs: []string{"party-1"}},
	}
	lobby, _ := store.CreateFromMatch("match-1", "1v1", teams, nil, nil, nil, now)

	closed, err := store.RemoveParty(lobby.MatchID, "party-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !closed {
		t.Error("expected lobby to be closed")
	}

	_, ok := store.Get(lobby.MatchID)
	if ok {
		t.Error("expected lobby to be deleted")
	}
}

func TestLobbyStore_SetState(t *testing.T) {
	store := NewLobbyStore()
	now := time.Now()

	teams := []types.LobbyTeam{
		{Name: "Team1", PartyIDs: []string{"party-1"}},
	}
	lobby, _ := store.CreateFromMatch("match-1", "1v1", teams, nil, nil, nil, now)

	err := store.SetState(lobby.MatchID, types.LobbyStateReady)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	lobby, _ = store.Get(lobby.MatchID)
	if lobby.State != types.LobbyStateReady {
		t.Errorf("expected state ready, got %s", lobby.State)
	}

	// Verify state index
	readyLobbies := store.GetByState(types.LobbyStateReady)
	if len(readyLobbies) != 1 {
		t.Errorf("expected 1 ready lobby, got %d", len(readyLobbies))
	}

	waitingLobbies := store.GetByState(types.LobbyStateWaiting)
	if len(waitingLobbies) != 0 {
		t.Errorf("expected 0 waiting lobbies, got %d", len(waitingLobbies))
	}
}

func TestLobbyStore_GetByState(t *testing.T) {
	store := NewLobbyStore()
	now := time.Now()

	teams1 := []types.LobbyTeam{{Name: "Team1", PartyIDs: []string{"party-1"}}}
	teams2 := []types.LobbyTeam{{Name: "Team1", PartyIDs: []string{"party-2"}}}

	store.CreateFromMatch("match-1", "1v1", teams1, nil, nil, nil, now)
	store.CreateFromMatch("match-2", "1v1", teams2, nil, nil, nil, now)

	waiting := store.GetByState(types.LobbyStateWaiting)
	if len(waiting) != 2 {
		t.Errorf("expected 2 waiting lobbies, got %d", len(waiting))
	}

	// No lobbies in other states
	ready := store.GetByState(types.LobbyStateReady)
	if len(ready) != 0 {
		t.Errorf("expected 0 ready lobbies, got %d", len(ready))
	}
}

func TestLobbyStore_CountByState(t *testing.T) {
	store := NewLobbyStore()
	now := time.Now()

	teams1 := []types.LobbyTeam{{Name: "Team1", PartyIDs: []string{"party-1"}}}
	teams2 := []types.LobbyTeam{{Name: "Team1", PartyIDs: []string{"party-2"}}}

	store.CreateFromMatch("match-1", "1v1", teams1, nil, nil, nil, now)
	lobby2, _ := store.CreateFromMatch("match-2", "1v1", teams2, nil, nil, nil, now)

	if store.CountByState(types.LobbyStateWaiting) != 2 {
		t.Errorf("expected 2 waiting, got %d", store.CountByState(types.LobbyStateWaiting))
	}

	store.SetState(lobby2.MatchID, types.LobbyStateReady)

	if store.CountByState(types.LobbyStateWaiting) != 1 {
		t.Errorf("expected 1 waiting, got %d", store.CountByState(types.LobbyStateWaiting))
	}
	if store.CountByState(types.LobbyStateReady) != 1 {
		t.Errorf("expected 1 ready, got %d", store.CountByState(types.LobbyStateReady))
	}
}

func TestLobbyStore_UpdateHeartbeat(t *testing.T) {
	store := NewLobbyStore()
	now := time.Now()

	teams := []types.LobbyTeam{
		{Name: "Team1", PartyIDs: []string{"party-1"}},
	}
	lobby, _ := store.CreateFromMatch("match-1", "1v1", teams, nil, nil, nil, now)

	heartbeatTime := now.Add(5 * time.Minute)
	err := store.UpdateHeartbeat(lobby.MatchID, heartbeatTime)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	lobby, _ = store.Get(lobby.MatchID)
	if lobby.LastHeartbeat == nil {
		t.Error("expected heartbeat to be set")
	}
	if !lobby.LastHeartbeat.Equal(heartbeatTime) {
		t.Errorf("expected heartbeat time %v, got %v", heartbeatTime, *lobby.LastHeartbeat)
	}
}

func TestLobbyStore_GetZombieLobbies(t *testing.T) {
	store := NewLobbyStore()
	now := time.Now()
	timeout := 15 * time.Minute

	// Create lobby, set to in_game with old start time
	teams := []types.LobbyTeam{
		{Name: "Team1", PartyIDs: []string{"party-1"}},
	}
	lobby, _ := store.CreateFromMatch("match-1", "1v1", teams, nil, nil, nil, now)
	store.SetState(lobby.MatchID, types.LobbyStateInGame)
	oldStart := now.Add(-20 * time.Minute)
	store.SetStartedAt(lobby.MatchID, oldStart)

	zombies := store.GetZombieLobbies(now, timeout)
	if len(zombies) != 1 {
		t.Errorf("expected 1 zombie lobby, got %d", len(zombies))
	}

	// Update heartbeat - should no longer be zombie
	store.UpdateHeartbeat(lobby.MatchID, now)
	zombies = store.GetZombieLobbies(now, timeout)
	if len(zombies) != 0 {
		t.Errorf("expected 0 zombie lobbies after heartbeat, got %d", len(zombies))
	}

	// Move time forward past timeout
	futureTime := now.Add(20 * time.Minute)
	zombies = store.GetZombieLobbies(futureTime, timeout)
	if len(zombies) != 1 {
		t.Errorf("expected 1 zombie lobby after timeout, got %d", len(zombies))
	}
}

func TestLobbyStore_Delete(t *testing.T) {
	store := NewLobbyStore()
	now := time.Now()

	teams := []types.LobbyTeam{
		{Name: "Team1", PartyIDs: []string{"party-1"}},
	}
	lobby, _ := store.CreateFromMatch("match-1", "1v1", teams, nil, nil, nil, now)

	ok := store.Delete(lobby.MatchID)
	if !ok {
		t.Error("expected delete to succeed")
	}

	_, found := store.Get(lobby.MatchID)
	if found {
		t.Error("expected lobby to be deleted")
	}

	// Party index should be cleared
	_, found = store.GetByParty("party-1")
	if found {
		t.Error("expected party index to be cleared")
	}
}

func TestLobbyStore_Clear(t *testing.T) {
	store := NewLobbyStore()
	now := time.Now()

	teams1 := []types.LobbyTeam{{Name: "Team1", PartyIDs: []string{"party-1"}}}
	teams2 := []types.LobbyTeam{{Name: "Team1", PartyIDs: []string{"party-2"}}}

	store.CreateFromMatch("match-1", "1v1", teams1, nil, nil, nil, now)
	store.CreateFromMatch("match-2", "1v1", teams2, nil, nil, nil, now)

	store.Clear()

	if store.Count() != 0 {
		t.Errorf("expected count 0, got %d", store.Count())
	}
}

func TestLobbyStore_RestoreAndCounter(t *testing.T) {
	store := NewLobbyStore()
	now := time.Now()

	teams := []types.LobbyTeam{
		{Name: "Team1", PartyIDs: []string{"party-1"}},
	}
	lobby, _ := store.CreateFromMatch("match-1", "1v1", teams, nil, nil, nil, now)
	store.SetState(lobby.MatchID, types.LobbyStateInGame)

	// Get all for snapshot
	lobbies := store.All()
	counter := store.GetCounter()

	// Clear and restore
	store.Clear()
	store.SetCounter(counter)
	for _, l := range lobbies {
		store.Restore(l)
	}

	// Verify restoration
	if store.Count() != 1 {
		t.Errorf("expected count 1, got %d", store.Count())
	}

	restored, ok := store.Get(lobby.MatchID)
	if !ok {
		t.Error("expected to find restored lobby")
	}
	if restored.MatchID != "match-1" {
		t.Errorf("expected match ID match-1, got %s", restored.MatchID)
	}
	if restored.State != types.LobbyStateInGame {
		t.Errorf("expected state in_game, got %s", restored.State)
	}

	// Verify indexes restored
	_, ok = store.GetByParty("party-1")
	if !ok {
		t.Error("expected party index to be restored")
	}

	inGame := store.GetByState(types.LobbyStateInGame)
	if len(inGame) != 1 {
		t.Errorf("expected 1 in_game lobby, got %d", len(inGame))
	}
}
