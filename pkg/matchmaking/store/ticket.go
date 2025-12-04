// Package store provides storage implementations for matchmaking.
package store

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/pkg/matchmaking/types"
)

// TicketStore manages tickets with multiple indexes for efficient access.
type TicketStore struct {
	mu sync.RWMutex

	// Primary storage - O(1) lookup by ID
	ticketsByID map[string]*types.Ticket

	// Index by match_profile_name, tickets sorted by created_at (oldest first)
	ticketsByProfile map[string][]*types.Ticket

	// Index for backfill-eligible tickets per profile
	backfillTicketsByProfile map[string][]*types.Ticket

	// Index by party_id - to check if party already has a ticket
	ticketsByParty map[string]*types.Ticket

	// Counter for generating unique IDs
	ticketCounter uint64
}

// NewTicketStore creates a new ticket store.
func NewTicketStore() *TicketStore {
	return &TicketStore{
		ticketsByID:              make(map[string]*types.Ticket),
		ticketsByProfile:         make(map[string][]*types.Ticket),
		backfillTicketsByProfile: make(map[string][]*types.Ticket),
		ticketsByParty:           make(map[string]*types.Ticket),
		ticketCounter:            0,
	}
}

// Create creates a new ticket and adds it to the store.
func (s *TicketStore) Create(
	partyID string,
	matchProfileName string,
	allowBackfill bool,
	players []types.PlayerInfo,
	createdAt time.Time,
	ttl time.Duration,
	poolCounts map[string]int,
) (*types.Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if party already has a ticket
	if _, exists := s.ticketsByParty[partyID]; exists {
		return nil, eris.Errorf("party %q already has an active ticket", partyID)
	}

	s.ticketCounter++
	ticket := &types.Ticket{
		ID:               uuid.New().String(),
		PartyID:          partyID,
		MatchProfileName: matchProfileName,
		AllowBackfill:    allowBackfill,
		Players:          players,
		CreatedAt:        createdAt,
		ExpiresAt:        createdAt.Add(ttl),
		PoolCounts:       poolCounts,
	}

	// Add to primary storage
	s.ticketsByID[ticket.ID] = ticket

	// Add to profile index (maintain sorted order by created_at)
	s.ticketsByProfile[matchProfileName] = insertTicketSorted(
		s.ticketsByProfile[matchProfileName], ticket)

	// Add to backfill index if eligible
	if allowBackfill {
		s.backfillTicketsByProfile[matchProfileName] = insertTicketSorted(
			s.backfillTicketsByProfile[matchProfileName], ticket)
	}

	// Add to party index
	s.ticketsByParty[partyID] = ticket

	return ticket, nil
}

// Get retrieves a ticket by ID.
func (s *TicketStore) Get(ticketID string) (*types.Ticket, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ticket, ok := s.ticketsByID[ticketID]
	return ticket, ok
}

// GetByParty retrieves a ticket by party ID.
func (s *TicketStore) GetByParty(partyID string) (*types.Ticket, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ticket, ok := s.ticketsByParty[partyID]
	return ticket, ok
}

// Delete removes a ticket from the store.
func (s *TicketStore) Delete(ticketID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.deleteUnlocked(ticketID)
}

// deleteUnlocked removes a ticket without acquiring the lock (caller must hold lock).
func (s *TicketStore) deleteUnlocked(ticketID string) bool {
	ticket, ok := s.ticketsByID[ticketID]
	if !ok {
		return false
	}

	// Remove from primary storage
	delete(s.ticketsByID, ticketID)

	// Remove from profile index
	s.ticketsByProfile[ticket.MatchProfileName] = removeTicketByID(
		s.ticketsByProfile[ticket.MatchProfileName], ticketID)

	// Remove from backfill index
	if ticket.AllowBackfill {
		s.backfillTicketsByProfile[ticket.MatchProfileName] = removeTicketByID(
			s.backfillTicketsByProfile[ticket.MatchProfileName], ticketID)
	}

	// Remove from party index
	delete(s.ticketsByParty, ticket.PartyID)

	return true
}

// DeleteMultiple removes multiple tickets efficiently.
func (s *TicketStore) DeleteMultiple(ticketIDs []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, id := range ticketIDs {
		s.deleteUnlocked(id)
	}
}

// GetByProfile returns all tickets for a profile, sorted by created_at (oldest first).
func (s *TicketStore) GetByProfile(matchProfileName string) []*types.Ticket {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tickets := s.ticketsByProfile[matchProfileName]
	// Return a copy to prevent external modification
	result := make([]*types.Ticket, len(tickets))
	copy(result, tickets)
	return result
}

// GetBackfillEligible returns backfill-eligible tickets for a profile.
func (s *TicketStore) GetBackfillEligible(matchProfileName string) []*types.Ticket {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tickets := s.backfillTicketsByProfile[matchProfileName]
	result := make([]*types.Ticket, len(tickets))
	copy(result, tickets)
	return result
}

// ExpireTickets removes all expired tickets and returns the count removed.
func (s *TicketStore) ExpireTickets(now time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	var expiredIDs []string
	for id, ticket := range s.ticketsByID {
		if ticket.IsExpired(now) {
			expiredIDs = append(expiredIDs, id)
		}
	}

	for _, id := range expiredIDs {
		s.deleteUnlocked(id)
	}

	return len(expiredIDs)
}

// Count returns the total number of tickets.
func (s *TicketStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.ticketsByID)
}

// CountByProfile returns the number of tickets for a profile.
func (s *TicketStore) CountByProfile(matchProfileName string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.ticketsByProfile[matchProfileName])
}

// All returns all tickets (for snapshot serialization).
func (s *TicketStore) All() []*types.Ticket {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*types.Ticket, 0, len(s.ticketsByID))
	for _, t := range s.ticketsByID {
		result = append(result, t)
	}
	return result
}

// GetCounter returns the current ticket counter (for snapshot).
func (s *TicketStore) GetCounter() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ticketCounter
}

// SetCounter sets the ticket counter (for restore).
func (s *TicketStore) SetCounter(counter uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ticketCounter = counter
}

// Clear removes all tickets (for reset).
func (s *TicketStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ticketsByID = make(map[string]*types.Ticket)
	s.ticketsByProfile = make(map[string][]*types.Ticket)
	s.backfillTicketsByProfile = make(map[string][]*types.Ticket)
	s.ticketsByParty = make(map[string]*types.Ticket)
}

// Restore adds a ticket directly (for snapshot restore).
func (s *TicketStore) Restore(ticket *types.Ticket) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ticketsByID[ticket.ID] = ticket
	s.ticketsByProfile[ticket.MatchProfileName] = insertTicketSorted(
		s.ticketsByProfile[ticket.MatchProfileName], ticket)
	if ticket.AllowBackfill {
		s.backfillTicketsByProfile[ticket.MatchProfileName] = insertTicketSorted(
			s.backfillTicketsByProfile[ticket.MatchProfileName], ticket)
	}
	s.ticketsByParty[ticket.PartyID] = ticket
}

// insertTicketSorted inserts a ticket into a slice maintaining sorted order by CreatedAt.
func insertTicketSorted(tickets []*types.Ticket, ticket *types.Ticket) []*types.Ticket {
	// Find insertion point using binary search
	i := 0
	j := len(tickets)
	for i < j {
		mid := (i + j) / 2
		if tickets[mid].CreatedAt.Before(ticket.CreatedAt) {
			i = mid + 1
		} else {
			j = mid
		}
	}

	// Insert at position i
	tickets = append(tickets, nil)
	copy(tickets[i+1:], tickets[i:])
	tickets[i] = ticket
	return tickets
}

// removeTicketByID removes a ticket from a slice by ID.
func removeTicketByID(tickets []*types.Ticket, ticketID string) []*types.Ticket {
	for i, t := range tickets {
		if t.ID == ticketID {
			return append(tickets[:i], tickets[i+1:]...)
		}
	}
	return tickets
}
