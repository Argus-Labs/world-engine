package store

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/argus-labs/world-engine/pkg/lobby/types"
)

// PartyStore manages party storage with indexes for efficient lookups.
// From ADR-030:
//   - parties_by_id: Map<party_id, Party>
//   - party_by_player: Map<player_id, party_id>
type PartyStore struct {
	mu sync.RWMutex

	// Primary storage - O(1) lookup by ID
	partiesByID map[string]*types.Party

	// Index by player - find party by player ID
	partyByPlayer map[string]string

	// Counter for deterministic ID generation
	counter uint64
}

// NewPartyStore creates a new party store.
func NewPartyStore() *PartyStore {
	return &PartyStore{
		partiesByID:   make(map[string]*types.Party),
		partyByPlayer: make(map[string]string),
	}
}

// Create creates a new party with the given leader.
// Returns error if the leader already has a party.
func (s *PartyStore) Create(leaderID string, isOpen bool, maxSize int, now time.Time) (*types.Party, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if leader already has a party
	if _, exists := s.partyByPlayer[leaderID]; exists {
		return nil, fmt.Errorf("player %s already has an active party", leaderID)
	}

	s.counter++
	party := &types.Party{
		ID:        uuid.New().String(),
		LeaderID:  leaderID,
		Members:   []string{leaderID},
		IsOpen:    isOpen,
		MaxSize:   maxSize,
		CreatedAt: now,
	}

	s.partiesByID[party.ID] = party
	s.partyByPlayer[leaderID] = party.ID

	return party, nil
}

// Get returns a party by ID.
func (s *PartyStore) Get(partyID string) (*types.Party, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	party, ok := s.partiesByID[partyID]
	return party, ok
}

// GetByPlayer returns the party for a given player.
func (s *PartyStore) GetByPlayer(playerID string) (*types.Party, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	partyID, ok := s.partyByPlayer[playerID]
	if !ok {
		return nil, false
	}
	return s.partiesByID[partyID], true
}

// AddMember adds a player to an existing party.
// Returns error if party is full, closed, or player already in a party.
func (s *PartyStore) AddMember(partyID, playerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	party, ok := s.partiesByID[partyID]
	if !ok {
		return fmt.Errorf("party %s not found", partyID)
	}

	// Check if player already has a party
	if _, exists := s.partyByPlayer[playerID]; exists {
		return fmt.Errorf("player %s already has an active party", playerID)
	}

	// Check if party is open
	if !party.IsOpen {
		return fmt.Errorf("party %s is not open for joining", partyID)
	}

	// Check if party is full
	if party.IsFull() {
		return fmt.Errorf("party %s is full", partyID)
	}

	party.Members = append(party.Members, playerID)
	s.partyByPlayer[playerID] = partyID

	return nil
}

// RemoveMember removes a player from a party.
// If the player is the leader, promotes the next member or disbands if empty.
// Returns true if party was disbanded.
func (s *PartyStore) RemoveMember(partyID, playerID string) (disbanded bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	party, ok := s.partiesByID[partyID]
	if !ok {
		return false, fmt.Errorf("party %s not found", partyID)
	}

	if !party.HasMember(playerID) {
		return false, fmt.Errorf("player %s is not in party %s", playerID, partyID)
	}

	// Remove from members
	newMembers := make([]string, 0, len(party.Members)-1)
	for _, m := range party.Members {
		if m != playerID {
			newMembers = append(newMembers, m)
		}
	}
	party.Members = newMembers

	// Remove from index
	delete(s.partyByPlayer, playerID)

	// If party is empty, disband
	if len(party.Members) == 0 {
		delete(s.partiesByID, partyID)
		return true, nil
	}

	// If leader left, promote next member
	if party.LeaderID == playerID {
		party.LeaderID = party.Members[0]
	}

	return false, nil
}

// SetLeader changes the party leader.
func (s *PartyStore) SetLeader(partyID, newLeaderID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	party, ok := s.partiesByID[partyID]
	if !ok {
		return fmt.Errorf("party %s not found", partyID)
	}

	if !party.HasMember(newLeaderID) {
		return fmt.Errorf("player %s is not in party %s", newLeaderID, partyID)
	}

	party.LeaderID = newLeaderID
	return nil
}

// SetOpen sets whether the party is open for joining.
func (s *PartyStore) SetOpen(partyID string, isOpen bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	party, ok := s.partiesByID[partyID]
	if !ok {
		return fmt.Errorf("party %s not found", partyID)
	}

	party.IsOpen = isOpen
	return nil
}

// SetLobby sets the lobby ID for a party.
func (s *PartyStore) SetLobby(partyID, lobbyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	party, ok := s.partiesByID[partyID]
	if !ok {
		return fmt.Errorf("party %s not found", partyID)
	}

	party.LobbyID = lobbyID
	return nil
}

// SetReady sets whether the party is ready (only relevant when in a lobby).
func (s *PartyStore) SetReady(partyID string, isReady bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	party, ok := s.partiesByID[partyID]
	if !ok {
		return fmt.Errorf("party %s not found", partyID)
	}

	party.IsReady = isReady
	return nil
}

// Delete removes a party and all its member indexes.
func (s *PartyStore) Delete(partyID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	party, ok := s.partiesByID[partyID]
	if !ok {
		return false
	}

	// Remove all member indexes
	for _, m := range party.Members {
		delete(s.partyByPlayer, m)
	}

	delete(s.partiesByID, partyID)
	return true
}

// All returns all parties.
func (s *PartyStore) All() []*types.Party {
	s.mu.RLock()
	defer s.mu.RUnlock()

	parties := make([]*types.Party, 0, len(s.partiesByID))
	for _, p := range s.partiesByID {
		parties = append(parties, p)
	}
	return parties
}

// Count returns the total number of parties.
func (s *PartyStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.partiesByID)
}

// Clear removes all parties.
func (s *PartyStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.partiesByID = make(map[string]*types.Party)
	s.partyByPlayer = make(map[string]string)
}

// Restore adds a party to the store (used for snapshot restoration).
func (s *PartyStore) Restore(party *types.Party) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.partiesByID[party.ID] = party
	for _, m := range party.Members {
		s.partyByPlayer[m] = party.ID
	}
}

// CreateFromMatch creates a party with a specific ID and player list (from matchmaking).
// This is used when receiving a match from the Matchmaking Shard.
// Returns error if the party ID already exists or if any player is already in a party.
func (s *PartyStore) CreateFromMatch(partyID string, playerIDs []string, now time.Time) (*types.Party, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if party already exists
	if _, exists := s.partiesByID[partyID]; exists {
		return nil, fmt.Errorf("party %s already exists", partyID)
	}

	// Check if any player already has a party
	for _, playerID := range playerIDs {
		if _, exists := s.partyByPlayer[playerID]; exists {
			return nil, fmt.Errorf("player %s already has an active party", playerID)
		}
	}

	// Default leader to first player
	var leaderID string
	if len(playerIDs) > 0 {
		leaderID = playerIDs[0]
	}

	s.counter++
	party := &types.Party{
		ID:        partyID,
		LeaderID:  leaderID,
		Members:   playerIDs,
		IsOpen:    false, // Matchmade parties are not open for joining
		MaxSize:   len(playerIDs),
		CreatedAt: now,
	}

	s.partiesByID[party.ID] = party
	for _, playerID := range playerIDs {
		s.partyByPlayer[playerID] = party.ID
	}

	return party, nil
}

// GetCounter returns the current counter value.
func (s *PartyStore) GetCounter() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.counter
}

// SetCounter sets the counter value (used for snapshot restoration).
func (s *PartyStore) SetCounter(val uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter = val
}
