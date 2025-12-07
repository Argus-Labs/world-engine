# Matchmaking Shard

A distributed matchmaking shard that implements `micro.ShardEngine` for deterministic replay and state management.

## Data Structure

### Core Types (`types/`)

| Type | Description |
|------|-------------|
| `Profile` | Match profile configuration: pools, team structure, addresses |
| `Pool` | Filter criteria to categorize players (region, elo range, tags) |
| `Ticket` | Matchmaking request for a party of players |
| `PlayerInfo` | Single player with `SearchFields` for filter matching |
| `SearchFields` | Player attributes: `StringArgs`, `DoubleArgs`, `Tags` |
| `Match` | Successful match result with teams and assigned tickets |
| `MatchTeam` | Team within a match containing tickets |
| `BackfillRequest` | Request to fill vacant slots in existing match |
| `BackfillMatch` | Successful backfill result |

### Stores (`store/`)

| Store | Indexes | Description |
|-------|---------|-------------|
| `TicketStore` | `ticketsByID`, `ticketsByProfile`, `backfillTicketsByProfile`, `ticketsByPlayer` | Manages tickets with O(1) lookup, sorted by `created_at` |
| `BackfillStore` | `backfillsByID`, `backfillsByProfile` | Manages backfill requests |
| `ProfileStore` | `profiles` (by name) | Holds match profile configurations |

### Public API

| Function | Description |
|----------|-------------|
| `NewWorld()` | Create new matchmaking shard instance |
| `World.StartMatchmaking()` | Start the matchmaking shard |
| `store.LoadProfilesFromFile()` | Load match profiles from JSON file |
| `store.LoadProfilesFromJSON()` | Load match profiles from JSON bytes |

### NATS Endpoints (External)

| Endpoint | Direction | Description |
|----------|-----------|-------------|
| `<address>.command.create-ticket` | Game → Matchmaking | Submit ticket to queue |
| `<address>.command.cancel-ticket` | Game → Matchmaking | Cancel existing ticket |
| `<address>.command.create-backfill` | Lobby → Matchmaking | Request backfill for match |
| `<address>.command.cancel-backfill` | Lobby → Matchmaking | Cancel backfill request |

### Callbacks (Matchmaking → External)

| Callback | Direction | Description |
|----------|-----------|-------------|
| `<callback>.matchmaking.ticket-created` | Matchmaking → Game | Ticket created successfully |
| `<callback>.matchmaking.ticket-error` | Matchmaking → Game | Ticket creation failed |
| `<callback>.matchmaking.match` | Matchmaking → Game | Match found for ticket |
| `<lobby>.matchmaking.match` | Matchmaking → Lobby | New match to create session |
| `<lobby>.matchmaking.backfill-match` | Matchmaking → Lobby | Backfill slots filled |

## Algorithm

The matchmaking algorithm uses bounded Dynamic Programming to assign tickets to teams:

1. **Sorting**: Candidates are sorted by `created_at` (oldest first), with player ID as tiebreaker for equal timestamps
2. **State Exploration**: DP explores valid team assignments respecting size and role constraints
3. **Deterministic Selection**: When multiple states have equal priority (same total wait time), the algorithm uses player ID ordering as tiebreaker
4. **Output Ordering**: Tickets within each team are sorted alphabetically by first player ID

This ensures fully deterministic results for replay, even when tickets are created in the same tick (same timestamp).

## Matchmaking Flow

### Ticket Lifecycle

**Step 1** Submit Ticket
- Game Shard submits ticket request with `party_id` (correlation ID) and `callback_address`
- Matchmaking Shard generates `ticket_id` and returns it to Game Shard via callback
- Both operations are async (fire-and-forget with callback)

**Step 2a** Match Found
- Matchmaking algorithm runs each tick, finds compatible tickets, forms a match
- Matchmaking publishes `Match` to Lobby Shard (for game session creation) and Game Shard(s) (via `callback_address`, for player notification)
- Matched tickets are removed from the queue

**Step 2b** Cancel Ticket
- Game Shard sends cancel request with `ticket_id`
- Matchmaking removes ticket from queue if it exists
- Fire-and-forget, no callback needed

**Step 2c** Ticket Expires
- Each ticket has TTL set by Game Shard via `ttl_seconds`
- Matchmaking automatically removes expired tickets each tick
- No notification sent, Game Shard should implement its own timeout handling

### Backfill Lifecycle

**Step 3** Create Backfill Request
- Lobby Shard detects player left mid-game, needs replacement
- Lobby sends backfill request with `match_id`, `team_name`, `slots_needed`, and `lobby_address`
- Matchmaking stores backfill request and prioritizes matching backfill-eligible tickets

**Step 4a** Backfill Match Found
- Matchmaking finds tickets that can fill the backfill slots
- Matchmaking publishes `BackfillMatch` to Lobby Shard (via `lobby_address`)
- Matched tickets are removed from queue, backfill request is fulfilled

**Step 4b** Cancel Backfill
- Lobby Shard cancels backfill request (e.g., game ended, slot filled by rejoin)
- Matchmaking removes backfill request if exists
- Fire-and-forget, no callback needed

**Step 4c** Backfill Expires
- Each backfill request has TTL set by Matchmaking config
- Matchmaking automatically removes expired backfill requests each tick
- No notification sent to Lobby

---

## Step 1: Submit Ticket

**Direction**: Game Shard → Matchmaking Shard → Game Shard (callback)

**Endpoint**: `<matchmaking-address>.command.create-ticket`

**Callbacks**:
- `<callback_address>.matchmaking.ticket-created` (success)
- `<callback_address>.matchmaking.ticket-error` (error)

### Game Shard Logic

**On Queue Request:**
- Generate `party_id` (UUID) for callback correlation
- Store `PendingTicket` keyed by `party_id`
- Send `CreateTicketCommand` to Matchmaking (fire-and-forget)

**On Ticket Created Callback:**
- Lookup `PendingTicket` by `party_id`, ignore if not found
- Move to `ActiveTicket` keyed by `ticket_id`
- Notify players: "You are now in queue"

**On Ticket Error Callback:**
- Lookup `PendingTicket` by `party_id`, ignore if not found
- Remove from pending, notify players with error

### Matchmaking Shard Logic

**On Create Ticket Command (during Tick):**
- Validate `match_profile_name` exists → if not, publish error callback
- Generate `ticket_id` (UUID)
- Create `Ticket` with `expires_at` = tick timestamp + `ttl_seconds`
- Store ticket, publish success callback with `{ party_id, ticket_id }`

---

## Step 2a: Match Found

**Direction**: Matchmaking Shard → Lobby Shard + Game Shard(s)

**Endpoints**:
- `<lobby_address>.matchmaking.match` (to Lobby)
- `<callback_address>.matchmaking.match` (to each Game Shard)

### Matchmaking Shard Logic

**Each Tick:**
- Run matchmaking algorithm on queued tickets
- When match formed, create `Match` with teams and ticket assignments
- Remove matched tickets from queue
- Publish `Match` to Lobby Shard (for game session creation)
- Publish `Match` to each unique `callback_address` (for player notification)

### Game Shard Logic

**On Match Callback:**
- Lookup `ActiveTicket` by `ticket_id` from match
- Remove from active tickets
- Notify players: "Match found" with opponent info and team assignments

### Lobby Shard Logic

**On Match Callback:**
- Create game session using `Match` data
- Use `target_address` (from `MatchProfile.TargetAddress`) to communicate with game server

---

## Step 2b: Cancel Ticket

**Direction**: Game Shard → Matchmaking Shard

**Endpoint**: `<matchmaking-address>.command.cancel-ticket`

### Game Shard Logic

**On Cancel Request:**
- Send `CancelTicketCommand` with `ticket_id` (fire-and-forget)
- Remove from `ActiveTicket` locally
- Notify players: "Queue cancelled"

### Matchmaking Shard Logic

**On Cancel Ticket Command (during Tick):**
- Remove ticket from queue if exists
- No callback sent

---

## Step 2c: Ticket Expires

**Direction**: Internal (Matchmaking Shard only)

### Matchmaking Shard Logic

**Each Tick:**
- Check all tickets against current tick timestamp
- Remove tickets where `expires_at` < tick timestamp
- No callback sent to Game Shard

### Game Shard Logic

**Recommended:**
- Implement local timeout based on `ttl_seconds`
- If no match callback received within TTL, notify players: "Queue timed out"

---

## FUTURE IMPROVEMENT: Cross-Shard Matching

Currently, `Match.TargetAddress` comes from `MatchProfile.TargetAddress`, meaning all matches for a profile go to the same Game Shard. This works when:
- All players connect to the same Game Shard, OR
- Players connect to regional Game Shards with separate NATS clusters

To support true cross-shard matching (players from different Game Shards matched together):

### Changes Required

1. **Matchmaking: Notify all originating Game Shards**
   - When match is formed, iterate all tickets and collect unique `callback_address` values
   - Publish `Match` to each unique Game Shard (in addition to Lobby)
   - Each originating Game Shard receives notification: "Your players were matched, game hosted on TargetAddress"

2. **Game Shard: Handle cross-shard notification**
   - On receiving match callback, check if `target_address` matches self
   - If yes: prepare to host the game
   - If no: notify players to connect to the target Game Shard (provide connection info)

3. **Client: Support shard migration**
   - Client needs ability to disconnect from current shard and connect to target shard
   - Requires connection info (IP/port or URL) in the match notification
