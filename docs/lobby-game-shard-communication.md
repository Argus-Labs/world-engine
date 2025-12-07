# Lobby ↔ Game Shard Communication

## Q1: Gameplay Shard Handoff

**Question:** When matchmaking completes and players are ready, how does Lobby Shard hand off the match to Game Shard?

**Answer:** Lobby Shard sends a `StartGameRequest` to Game Shard via NATS.

### Flow

1. Matchmaking Shard finds a match and sends it to Lobby Shard
2. Lobby Shard creates a lobby and runs ready-check
3. When all players are ready, Lobby Shard sends `StartGameRequest` to Game Shard's `target_address`
4. Game Shard receives the request and initializes the game session

### Message

```protobuf
message StartGameRequest {
  string match_id = 1;
  string profile_name = 2;
  repeated TeamInfo teams = 3;
  google.protobuf.Struct config = 4;
  micro.v1.ServiceAddress lobby_address = 5;
}

message TeamInfo {
  string name = 1;
  repeated string player_ids = 2;
}
```

### Sample Data

```json
{
  "match_id": "match-abc123",
  "profile_name": "5v5-ranked",
  "teams": [
    {
      "name": "Red",
      "player_ids": ["player-1", "player-2", "player-3", "player-4", "player-5"]
    },
    {
      "name": "Blue",
      "player_ids": ["player-6", "player-7", "player-8", "player-9", "player-10"]
    }
  ],
  "config": {
    "map": "dust2",
    "mode": "competitive"
  },
  "lobby_address": {
    "namespace": "lobby",
    "shard_id": "lobby-shard-1"
  }
}
```

---

## Q2: Connection Info

**Question:** How do players receive connection information to join the game?

**Answer:** The original design assumed we needed to return `connection_info` from Game Shard to tell players where to connect. This is incorrect because all players are already connected to Game Shard before matchmaking even begins.

Since players are already connected to Game Shard, we simply send the match data via `StartGameRequest`. Game Shard receives this data and notifies its already-connected players directly through its own client communication mechanism (WebSocket, etc.).

Note: We use `match_id` as the primary identifier. There is no separate `lobby_id` because Lobby Shard uses `match_id` directly as its key - having two IDs adds unnecessary complexity with no benefit.

---

## Q3: Backfilled Players Notification

**Question:** How are backfilled players notified when they are matched to an existing game?

**Answer:** Lobby Shard sends a `BackfillNotification` to Game Shard when backfill players are matched. Game Shard then notifies its connected players directly.

### Flow

1. Game Shard detects it needs more players (e.g., player left mid-game)
2. Game Shard sends `RequestBackfillRequest` to Lobby Shard
3. Lobby Shard forwards the request to Matchmaking Shard
4. Matchmaking Shard finds players to fill the slots
5. Matchmaking Shard sends `BackfillMatch` to Lobby Shard
6. Lobby Shard forwards `BackfillNotification` to Game Shard
7. Game Shard adds new players to the game and notifies them directly

### Message

```protobuf
message BackfillNotification {
  string backfill_request_id = 1;
  string match_id = 2;
  string team_name = 3;
  repeated string player_ids = 4;
  micro.v1.ServiceAddress lobby_address = 5;
}
```

### Sample Data

```json
{
  "backfill_request_id": "backfill-xyz789",
  "match_id": "match-abc123",
  "team_name": "Red",
  "player_ids": ["player-11", "player-12"],
  "lobby_address": {
    "namespace": "lobby",
    "shard_id": "lobby-shard-1"
  }
}
```

---

## Q4: Lobby Shard Functions

**Question:** What endpoints does Lobby Shard expose for Game Shard to call?

**Answer:** Lobby Shard exposes endpoints for heartbeat, player status, end-match, and backfill management. All endpoints use `match_id` as the primary identifier.

### Heartbeat

Keep the lobby alive while game is in progress. Game Shard should send heartbeats periodically to prevent the lobby from being cleaned up as a zombie.

```protobuf
message HeartbeatRequest {
  string match_id = 1;
}
```

Sample:
```json
{
  "match_id": "match-abc123"
}
```

### Player Status

Report player connection status changes. Used to track disconnects and reconnects during gameplay.

```protobuf
message PlayerStatusRequest {
  string match_id = 1;
  string player_id = 2;
  PlayerStatus status = 3;
}

enum PlayerStatus {
  PLAYER_STATUS_UNSPECIFIED = 0;
  PLAYER_STATUS_CONNECTED = 1;
  PLAYER_STATUS_DISCONNECTED = 2;
  PLAYER_STATUS_RECONNECTED = 3;
}
```

Sample:
```json
{
  "match_id": "match-abc123",
  "player_id": "player-3",
  "status": "PLAYER_STATUS_DISCONNECTED"
}
```

### End Match

Signal that the game has ended. Lobby Shard will clean up the lobby and notify Matchmaking Shard to release any pending backfill requests.

```protobuf
message EndMatchRequest {
  string match_id = 1;
  MatchResult result = 2;
}

enum MatchResult {
  MATCH_RESULT_UNSPECIFIED = 0;
  MATCH_RESULT_COMPLETED = 1;
  MATCH_RESULT_CANCELLED = 2;
  MATCH_RESULT_ABANDONED = 3;
}
```

Sample:
```json
{
  "match_id": "match-abc123",
  "result": "MATCH_RESULT_COMPLETED"
}
```

### Request Backfill

Request additional players for a running game. Lobby Shard forwards this to Matchmaking Shard.

```protobuf
message RequestBackfillRequest {
  string match_id = 1;
  string profile_name = 2;
  string team_name = 3;
  repeated BackfillSlotNeeded slots_needed = 4;
}

message BackfillSlotNeeded {
  string pool_name = 1;
  int32 count = 2;
}
```

Sample:
```json
{
  "match_id": "match-abc123",
  "profile_name": "5v5-ranked",
  "team_name": "Red",
  "slots_needed": [
    {"pool_name": "dps", "count": 1},
    {"pool_name": "support", "count": 1}
  ]
}
```

### Cancel Backfill

Cancel a pending backfill request (e.g., player reconnected before backfill was fulfilled).

```protobuf
message CancelBackfillRequest {
  string backfill_request_id = 1;
}
```

Sample:
```json
{
  "backfill_request_id": "backfill-xyz789"
}
```

---

## Q5: Player Disconnect Handling

**Question:** How are player disconnects during a game handled?

**Answer:** Game Shard reports player status changes to Lobby Shard via `PlayerStatusRequest`. Lobby Shard tracks disconnected players and can trigger backfill or match abandonment based on configured policies.

### Flow

1. Player disconnects from Game Shard
2. Game Shard sends `PlayerStatusRequest` with `PLAYER_STATUS_DISCONNECTED` to Lobby Shard
3. Lobby Shard tracks the disconnected player in `DisconnectedParties`
4. (Optional) Game Shard requests backfill if needed
5. If player reconnects, Game Shard sends `PLAYER_STATUS_RECONNECTED`
6. Lobby Shard removes player from disconnected list

### Sample Disconnect

```json
{
  "match_id": "match-abc123",
  "player_id": "player-3",
  "status": "PLAYER_STATUS_DISCONNECTED"
}
```

### Sample Reconnect

```json
{
  "match_id": "match-abc123",
  "player_id": "player-3",
  "status": "PLAYER_STATUS_RECONNECTED"
}
```

### Use Cases

- **Analytics**: Track player disconnect rates and patterns
- **Backfill Triggering**: Automatically request backfill when too many players disconnect
- **Match Abandonment**: Cancel match if all players disconnect
- **Reconnection Window**: Allow players to rejoin within a grace period
