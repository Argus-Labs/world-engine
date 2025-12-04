package store

import (
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/argus-labs/world-engine/pkg/matchmaking/types"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
)

// BackfillStore manages backfill requests.
type BackfillStore struct {
	mu sync.RWMutex

	// Primary storage by ID
	requestsByID map[string]*types.BackfillRequest

	// Index by match_id (a match can have multiple backfill requests)
	requestsByMatch map[string][]*types.BackfillRequest

	// Counter for generating unique IDs
	backfillCounter uint64
}

// NewBackfillStore creates a new backfill store.
func NewBackfillStore() *BackfillStore {
	return &BackfillStore{
		requestsByID:    make(map[string]*types.BackfillRequest),
		requestsByMatch: make(map[string][]*types.BackfillRequest),
		backfillCounter: 0,
	}
}

// Create creates a new backfill request.
func (s *BackfillStore) Create(
	matchID string,
	matchProfileName string,
	teamName string,
	slotsNeeded []types.SlotNeeded,
	lobbyAddress *microv1.ServiceAddress,
	createdAt time.Time,
	ttl time.Duration,
) *types.BackfillRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.backfillCounter++
	req := &types.BackfillRequest{
		ID:               uuid.New().String(),
		MatchID:          matchID,
		MatchProfileName: matchProfileName,
		TeamName:         teamName,
		SlotsNeeded:      slotsNeeded,
		LobbyAddress:     lobbyAddress,
		CreatedAt:        createdAt,
		ExpiresAt:        createdAt.Add(ttl),
	}

	s.requestsByID[req.ID] = req
	s.requestsByMatch[matchID] = append(s.requestsByMatch[matchID], req)

	return req
}

// Get retrieves a backfill request by ID.
func (s *BackfillStore) Get(id string) (*types.BackfillRequest, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	req, ok := s.requestsByID[id]
	return req, ok
}

// GetByMatch retrieves all backfill requests for a match.
func (s *BackfillStore) GetByMatch(matchID string) []*types.BackfillRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()

	reqs := s.requestsByMatch[matchID]
	result := make([]*types.BackfillRequest, len(reqs))
	copy(result, reqs)
	return result
}

// Delete removes a backfill request.
func (s *BackfillStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.deleteUnlocked(id)
}

func (s *BackfillStore) deleteUnlocked(id string) bool {
	req, ok := s.requestsByID[id]
	if !ok {
		return false
	}

	delete(s.requestsByID, id)

	// Remove from match index
	reqs := s.requestsByMatch[req.MatchID]
	for i, r := range reqs {
		if r.ID == id {
			s.requestsByMatch[req.MatchID] = append(reqs[:i], reqs[i+1:]...)
			break
		}
	}
	if len(s.requestsByMatch[req.MatchID]) == 0 {
		delete(s.requestsByMatch, req.MatchID)
	}

	return true
}

// All returns all backfill requests (ordered by creation time).
func (s *BackfillStore) All() []*types.BackfillRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*types.BackfillRequest, 0, len(s.requestsByID))
	for _, req := range s.requestsByID {
		result = append(result, req)
	}
	return result
}

// ExpireRequests removes expired backfill requests.
func (s *BackfillStore) ExpireRequests(now time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	var expiredIDs []string
	for id, req := range s.requestsByID {
		if req.IsExpired(now) {
			expiredIDs = append(expiredIDs, id)
		}
	}

	for _, id := range expiredIDs {
		s.deleteUnlocked(id)
	}

	return len(expiredIDs)
}

// Count returns the total number of backfill requests.
func (s *BackfillStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.requestsByID)
}

// GetCounter returns the current counter (for snapshot).
func (s *BackfillStore) GetCounter() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.backfillCounter
}

// SetCounter sets the counter (for restore).
func (s *BackfillStore) SetCounter(counter uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backfillCounter = counter
}

// Clear removes all backfill requests.
func (s *BackfillStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.requestsByID = make(map[string]*types.BackfillRequest)
	s.requestsByMatch = make(map[string][]*types.BackfillRequest)
}

// Restore adds a backfill request directly (for snapshot restore).
func (s *BackfillStore) Restore(req *types.BackfillRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.requestsByID[req.ID] = req
	s.requestsByMatch[req.MatchID] = append(s.requestsByMatch[req.MatchID], req)
}
