package component

// TicketIndexComponent provides O(1) lookups for tickets.
// This is a singleton component - only one entity should have it.
type TicketIndexComponent struct {
	// TicketIDToEntity maps TicketID -> EntityID for O(1) lookup
	TicketIDToEntity map[string]uint32 `json:"ticket_id_to_entity"`

	// PlayerToTicket maps PlayerID -> TicketID (one ticket per player)
	PlayerToTicket map[string]string `json:"player_to_ticket"`

	// ProfileToTickets maps ProfileName -> []TicketID
	ProfileToTickets map[string][]string `json:"profile_to_tickets"`

	// BackfillEligible maps ProfileName -> []TicketID (tickets that allow backfill)
	BackfillEligible map[string][]string `json:"backfill_eligible"`
}

// Name returns the component name for ECS registration.
func (TicketIndexComponent) Name() string { return "matchmaking_ticket_index" }

// Init initializes the maps if nil.
func (idx *TicketIndexComponent) Init() {
	if idx.TicketIDToEntity == nil {
		idx.TicketIDToEntity = make(map[string]uint32)
	}
	if idx.PlayerToTicket == nil {
		idx.PlayerToTicket = make(map[string]string)
	}
	if idx.ProfileToTickets == nil {
		idx.ProfileToTickets = make(map[string][]string)
	}
	if idx.BackfillEligible == nil {
		idx.BackfillEligible = make(map[string][]string)
	}
}

// GetEntityID returns the entity ID for a ticket.
func (idx *TicketIndexComponent) GetEntityID(ticketID string) (uint32, bool) {
	eid, exists := idx.TicketIDToEntity[ticketID]
	return eid, exists
}

// GetTicketByPlayer returns the ticket ID for a player.
func (idx *TicketIndexComponent) GetTicketByPlayer(playerID string) (string, bool) {
	ticketID, exists := idx.PlayerToTicket[playerID]
	return ticketID, exists
}

// HasPlayer checks if a player already has a ticket.
func (idx *TicketIndexComponent) HasPlayer(playerID string) bool {
	_, exists := idx.PlayerToTicket[playerID]
	return exists
}

// GetTicketsByProfile returns all ticket IDs for a profile.
func (idx *TicketIndexComponent) GetTicketsByProfile(profileName string) []string {
	return idx.ProfileToTickets[profileName]
}

// GetBackfillEligible returns backfill-eligible ticket IDs for a profile.
func (idx *TicketIndexComponent) GetBackfillEligible(profileName string) []string {
	return idx.BackfillEligible[profileName]
}

// AddTicket adds a ticket to the index.
func (idx *TicketIndexComponent) AddTicket(ticketID string, entityID uint32, profileName string, playerIDs []string, allowBackfill bool) {
	idx.Init()

	idx.TicketIDToEntity[ticketID] = entityID

	for _, playerID := range playerIDs {
		idx.PlayerToTicket[playerID] = ticketID
	}

	idx.ProfileToTickets[profileName] = append(idx.ProfileToTickets[profileName], ticketID)

	if allowBackfill {
		idx.BackfillEligible[profileName] = append(idx.BackfillEligible[profileName], ticketID)
	}
}

// RemoveTicket removes a ticket from the index.
func (idx *TicketIndexComponent) RemoveTicket(ticketID string, profileName string, playerIDs []string, allowBackfill bool) {
	delete(idx.TicketIDToEntity, ticketID)

	for _, playerID := range playerIDs {
		delete(idx.PlayerToTicket, playerID)
	}

	idx.ProfileToTickets[profileName] = removeFromSlice(idx.ProfileToTickets[profileName], ticketID)

	if allowBackfill {
		idx.BackfillEligible[profileName] = removeFromSlice(idx.BackfillEligible[profileName], ticketID)
	}
}

// ProfileIndexComponent provides O(1) lookups for profiles.
type ProfileIndexComponent struct {
	// ProfileNameToEntity maps ProfileName -> EntityID
	ProfileNameToEntity map[string]uint32 `json:"profile_name_to_entity"`
}

// Name returns the component name for ECS registration.
func (ProfileIndexComponent) Name() string { return "matchmaking_profile_index" }

// Init initializes the maps if nil.
func (idx *ProfileIndexComponent) Init() {
	if idx.ProfileNameToEntity == nil {
		idx.ProfileNameToEntity = make(map[string]uint32)
	}
}

// GetEntityID returns the entity ID for a profile.
func (idx *ProfileIndexComponent) GetEntityID(profileName string) (uint32, bool) {
	eid, exists := idx.ProfileNameToEntity[profileName]
	return eid, exists
}

// AddProfile adds a profile to the index.
func (idx *ProfileIndexComponent) AddProfile(profileName string, entityID uint32) {
	idx.Init()
	idx.ProfileNameToEntity[profileName] = entityID
}

// BackfillIndexComponent provides O(1) lookups for backfill requests.
type BackfillIndexComponent struct {
	// BackfillIDToEntity maps BackfillID -> EntityID
	BackfillIDToEntity map[string]uint32 `json:"backfill_id_to_entity"`

	// MatchToBackfill maps MatchID -> BackfillID
	MatchToBackfill map[string]string `json:"match_to_backfill"`

	// ProfileToBackfills maps ProfileName -> []BackfillID
	ProfileToBackfills map[string][]string `json:"profile_to_backfills"`
}

// Name returns the component name for ECS registration.
func (BackfillIndexComponent) Name() string { return "matchmaking_backfill_index" }

// Init initializes the maps if nil.
func (idx *BackfillIndexComponent) Init() {
	if idx.BackfillIDToEntity == nil {
		idx.BackfillIDToEntity = make(map[string]uint32)
	}
	if idx.MatchToBackfill == nil {
		idx.MatchToBackfill = make(map[string]string)
	}
	if idx.ProfileToBackfills == nil {
		idx.ProfileToBackfills = make(map[string][]string)
	}
}

// GetEntityID returns the entity ID for a backfill request.
func (idx *BackfillIndexComponent) GetEntityID(backfillID string) (uint32, bool) {
	eid, exists := idx.BackfillIDToEntity[backfillID]
	return eid, exists
}

// GetByMatch returns the backfill ID for a match.
func (idx *BackfillIndexComponent) GetByMatch(matchID string) (string, bool) {
	backfillID, exists := idx.MatchToBackfill[matchID]
	return backfillID, exists
}

// AddBackfill adds a backfill request to the index.
func (idx *BackfillIndexComponent) AddBackfill(backfillID string, entityID uint32, matchID string, profileName string) {
	idx.Init()
	idx.BackfillIDToEntity[backfillID] = entityID
	idx.MatchToBackfill[matchID] = backfillID
	idx.ProfileToBackfills[profileName] = append(idx.ProfileToBackfills[profileName], backfillID)
}

// RemoveBackfill removes a backfill request from the index.
func (idx *BackfillIndexComponent) RemoveBackfill(backfillID string, matchID string, profileName string) {
	delete(idx.BackfillIDToEntity, backfillID)
	delete(idx.MatchToBackfill, matchID)
	idx.ProfileToBackfills[profileName] = removeFromSlice(idx.ProfileToBackfills[profileName], backfillID)
}

// ConfigComponent stores matchmaking configuration.
type ConfigComponent struct {
	// LobbyTarget is the target shard for lobby commands.
	// If nil, same-shard communication via SystemEvents is used.
	LobbyShardID string `json:"lobby_shard_id,omitempty"`

	// DefaultTTLSeconds is the default ticket TTL.
	DefaultTTLSeconds int64 `json:"default_ttl_seconds"`

	// BackfillTTLSeconds is the default backfill request TTL.
	BackfillTTLSeconds int64 `json:"backfill_ttl_seconds"`
}

// Name returns the component name for ECS registration.
func (ConfigComponent) Name() string { return "matchmaking_config" }

// removeFromSlice removes a value from a slice.
func removeFromSlice(slice []string, value string) []string {
	for i, v := range slice {
		if v == value {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}
