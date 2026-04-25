# Lobby Plugin

A flexible lobby/party system for Cardinal worlds, handling player grouping and session management. Implemented as a Cardinal plugin.

## Quick Start

```go
import (
    "github.com/argus-labs/world-engine/pkg/cardinal"
    "github.com/argus-labs/world-engine/pkg/lobby"
)

func main() {
    world, _ := cardinal.NewWorld(cardinal.WorldOptions{...})

    cardinal.RegisterPlugin(world, lobby.NewPlugin(lobby.Config{
        LobbyWorld: cardinal.OtherWorld{
            Region:       "us-west",
            Organization: "myorg",
            Project:      "myproject",
            ShardID:      "lobby-shard-1",
        },
    }))

    world.StartGame()
}
```

## Overview

- **PvE**: Lobby acts as a persistent party that plays games together
- **PvP**: Lobby acts as a temporary session where multiple teams gather

## Message Categories

The lobby package uses 4 message categories:

### 1. Command (Client ↔ Shard)
Client sends a command to the shard. Shard processes it and returns a result to the sender.

| Command | Request Fields | Response Fields |
|---------|----------------|-----------------|
| `CreateLobby` | `Teams []TeamConfig`, `GameWorld`, `PlayerPassthroughData?`, `SessionPassthroughData?` | `IsSuccess`, `Message`, `Lobby` |
| `JoinLobby` | `InviteCode`, `TeamName?`, `PlayerPassthroughData?` | `IsSuccess`, `Message`, `Lobby` |
| `JoinTeam` | `TeamName` | `IsSuccess`, `Message` |
| `LeaveLobby` | - | `IsSuccess`, `Message` |
| `SetReady` | `IsReady` | `IsSuccess`, `Message` |
| `KickPlayer` | `TargetPlayerID` | `IsSuccess`, `Message` |
| `TransferLeader` | `TargetPlayerID` | `IsSuccess`, `Message` |
| `StartSession` | - | `IsSuccess`, `Message` |
| `GenerateInviteCode` | - | `IsSuccess`, `Message`, `InviteCode` |
| `UpdateSessionPassthrough` | `PassthroughData` | `IsSuccess`, `Message` |
| `UpdatePlayerPassthrough` | `PassthroughData` | `IsSuccess`, `Message` |
| `Heartbeat` | - | - (no response) |

All commands include `RequestID` for request/response correlation (except `Heartbeat`).

### 2. Event (Shard → All Clients, broadcast)
Shard broadcasts state changes to all subscribed clients.

| Event | Fields | Description |
|-------|--------|-------------|
| `LobbyCreatedEvent` | `LobbyID`, `LeaderID`, `InviteCode` | Lobby created |
| `PlayerJoinedEvent` | `LobbyID`, `TeamName`, `Player` (PlayerState) | Player joined |
| `PlayerLeftEvent` | `LobbyID`, `PlayerID` | Player left |
| `PlayerKickedEvent` | `LobbyID`, `PlayerID`, `KickerID` | Player kicked |
| `PlayerReadyEvent` | `LobbyID`, `PlayerID`, `IsReady` | Ready status changed |
| `PlayerChangedTeamEvent` | `LobbyID`, `PlayerID`, `OldTeamName`, `NewTeamName` | Player changed team |
| `LeaderChangedEvent` | `LobbyID`, `OldLeaderID`, `NewLeaderID` | Leadership transferred |
| `SessionStartedEvent` | `LobbyID` | Session started |
| `SessionEndedEvent` | `LobbyID` | Session ended |
| `SessionAwaitingAllocationEvent` | `LobbyID` | Session is pending shard assignment; orchestrators listen for this |
| `InviteCodeGeneratedEvent` | `LobbyID`, `InviteCode` | New code generated |
| `LobbyDeletedEvent` | `LobbyID` | Lobby deleted (empty) |
| `PlayerTimedOutEvent` | `LobbyID`, `PlayerID` | Player removed due to missed heartbeats |
| `SessionPassthroughUpdatedEvent` | `Lobby` | Session passthrough data updated |
| `PlayerPassthroughUpdatedEvent` | `Lobby`, `PlayerID` | Player passthrough data updated |

### 3. CrossShardCommand (Shard → Shard)
One shard sends a command to another shard.

| Command | Direction | Fields | Description |
|---------|-----------|--------|-------------|
| `NotifySessionStartCommand` | Lobby → Game | `Lobby`, `LobbyWorld` | Notify game shard to start session |
| `NotifySessionEndCommand` | Game → Lobby | `LobbyID` | Notify lobby shard to end session |
| `AssignShardCommand` | Orchestrator → Lobby | `LobbyID`, `RequestID`, `GameWorld`, `Reason` | Complete a pending session-start by assigning a game shard (see Shard Assignment) |

### 4. CrossShardResult (Shard → Shard, response)
Shard sends a response back to the originating shard.

*Currently not needed. CrossShardCommands in this package don't require responses.*

## Data Structures

### Lobby

```
Lobby
- ID string              // Unique identifier
- LeaderID string        // Player who controls the lobby
- Teams []Team           // List of teams
- InviteCode string      // Code for others to join
- GameWorld OtherWorld   // Target game shard address
- Session Session        // Current session state
- CreatedAt int64        // Unix timestamp
```

### Team

```
Team
- TeamID string          // Unique team identifier
- Name string            // Team display name
- Players []PlayerState  // Players in team
- MaxPlayers int         // Max players (0 = unlimited)
```

### PlayerState

```
PlayerState
- PlayerID string                 // Player identifier
- IsReady bool                    // Ready status
- PassthroughData map[string]any  // Forwarded to game shard
- JoinedAt int64                  // Unix timestamp when player joined
```

### Session

```
Session
- State SessionState                   // idle | awaiting_allocation | in_session
- PassthroughData map[string]any       // Forwarded to game shard
- PendingRequestID string              // StartSessionCommand RequestID while awaiting_allocation
- PendingStartedAt int64               // Unix seconds at which awaiting_allocation began
```

## Configuration

| Option | Description | Default |
|--------|-------------|---------|
| `LobbyWorld` | This lobby shard's address (for game shard to send NotifySessionEndCommand back) | required |
| `Provider` | Custom provider (optional, default provided) | `DefaultProvider` |
| `HeartbeatTimeout` | Seconds before a player is removed for not sending heartbeats. Clients should send heartbeats more frequently (e.g., every timeout/3 seconds). | 30 |
| `AssignmentAuthority` | Accident-prevention filter for `AssignShardCommand` — dropped when `cmd.Persona` differs. **Not authentication.** `cmd.Persona` is not signature-verified at this layer, so a forged client command still passes if it matches. Real auth (NATS ACLs, gateway auth) must live above the plugin. | empty |
| `MaxAllocationTimeout` | Max seconds a lobby may sit in `awaiting_allocation` before the plugin fails the pending request. `<= 0` disables timeout enforcement. | 0 (disabled) |

## Cross-Shard Communication

When `StartSessionCommand` succeeds, `NotifySessionStartCommand` is sent to game shard:

```go
type NotifySessionStartCommand struct {
    Lobby      LobbyComponent    // Full lobby data (includes GameWorld)
    LobbyWorld cardinal.OtherWorld // For NotifySessionEndCommand callback
}
```

Game shard should:
1. Register a system that handles `NotifySessionStartCommand`
2. Run the game
3. Send `NotifySessionEndCommand` back to lobby when game ends

```go
// In game shard
func GameSessionSystem(state *GameSessionSystemState) {
    for cmd := range state.SessionStartCmds.Iter() {
        payload := cmd.Payload
        // payload.Lobby contains full lobby data
        // payload.LobbyWorld for sending NotifySessionEndCommand back

        // When game ends:
        payload.LobbyWorld.SendCommand(&state.BaseSystemState, lobby.NotifySessionEndCommand{
            LobbyID: payload.Lobby.ID,
        })
    }
}
```

## Custom Provider

Override invite code generation:

```go
type MyProvider struct {
    lobby.DefaultProvider
}

func (p MyProvider) GenerateInviteCode(l *lobby.Component) string {
    return generateMyCustomCode(8)
}

cardinal.RegisterPlugin(world, lobby.NewPlugin(lobby.Config{
    LobbyWorld: cardinal.OtherWorld{...},
    Provider:   MyProvider{},
}))
```

Default: `Hash(LobbyID + Timestamp)` -> 6-char uppercase alphanumeric (excludes confusing chars: 0, O, I, L, 1).

## Lifecycle

```
CreateLobby -> idle -> (players join, ready up) -> StartSession -> awaiting_allocation
                                                                         │
                                                                         v
                                                                  AssignShardCommand
                                                                         │
                                                                         v
                                                                     in_session
                                                                         │
                                                                         v
                                                                    EndSession
                                                                         │
                                                                         v
                                                                       idle
                                                          (players can start again)
```

`StartSession` does not commit the session directly — the lobby enters
`awaiting_allocation` and emits `SessionAwaitingAllocationEvent`. An
external orchestrator (another system running in the same shard) picks a
game shard and sends `AssignShardCommand` back to complete the start.

## Shard Assignment

When a `StartSessionCommand` is accepted, the plugin does not pick a
game shard itself. Instead it transitions the lobby to
`awaiting_allocation` and waits for an orchestrator to send
`AssignShardCommand`. This lets consumers plug in any assignment strategy
(static, round-robin, probe + claim, external matchmaker, etc.) without
the lobby plugin knowing anything about the game.

### ⚠ If you enable this plugin, you MUST register an orchestrator

Every `StartSessionCommand` parks the lobby in
`awaiting_allocation` and waits for `AssignShardCommand`. Without an
orchestrator registered in the same shard, sessions never start and
clients sit waiting until `MaxAllocationTimeout` expires (or forever,
if unset). There is no implicit default behavior and no startup
detection — the plugin cannot tell whether an orchestrator exists.

**Always set `MaxAllocationTimeout` to a reasonable value** (e.g.,
30–60 seconds). This is your only safety net: if the orchestrator is
missing or broken, the timeout fails the pending session and returns
the lobby to idle instead of hanging forever.

### The orchestrator is just a normal system

An orchestrator is not a special plugin or interface. It is a regular
system registered in the same lobby shard.

### Copy-paste template

Drop this into your lobby shard. Change the five `REPLACE:` markers
and you're running. The rest is boilerplate — the system shape stays
the same regardless of how fancy your assignment logic becomes.

> **Heads up:** this minimal template has no in-flight tracking. It
> iterates every pending lobby each tick and sends
> `AssignShardCommand` unconditionally. The plugin's one-shot
> `RequestID` check rejects duplicates, but you'll see WARN logs for
> every redundant send until the lobby leaves
> `awaiting_allocation`. That's fine for the first smoke test or a
> one-server deployment; for anything real, track in-flight
> assignments — see the pool-based example below for the pattern
> (a `PendingProbe`-style component destroyed after the first send).

```go
package main

import (
    "github.com/argus-labs/world-engine/pkg/cardinal"
    "github.com/argus-labs/world-engine/pkg/lobby"
)

type AssignerState struct {
    cardinal.BaseSystemState
    Lobbies cardinal.Contains[struct {
        Lobby cardinal.Ref[lobby.Component]
    }]
}

func AssignerSystem(state *AssignerState) {
    // REPLACE: your lobby shard's own address.
    self := cardinal.OtherWorld{
        Region:       "us-west", // REPLACE
        Organization: "myorg",   // REPLACE
        Project:      "mygame",  // REPLACE
        ShardID:      "lobby",   // REPLACE
    }

    for _, refs := range state.Lobbies.Iter() {
        lob := refs.Lobby.Get()
        if lob.Session.State != lobby.SessionStateAwaitingAllocation {
            continue
        }
        self.SendCommand(&state.BaseSystemState, lobby.AssignShardCommand{
            LobbyID:   lob.ID,
            RequestID: lob.Session.PendingRequestID,
            GameWorld: cardinal.OtherWorld{
                Region:       "us-west", // REPLACE
                Organization: "myorg",   // REPLACE
                Project:      "mygame",  // REPLACE
                ShardID:      "game-shard-1", // REPLACE: pick your target shard
            },
        })
    }
}
```

Wire it up in `main.go` alongside the plugin:

```go
cardinal.RegisterPlugin(world, lobby.NewPlugin(lobby.Config{...}))
cardinal.RegisterSystem(world, AssignerSystem)
```

That's it. Every `StartSessionCommand` now routes to the shard you
named. To get smarter behavior (round-robin, probe-based idle
selection, external matchmaker), replace the hardcoded `ShardID` with
whatever selection logic you want — the command shape stays identical.

### Minimum requirements for the orchestrator

All three fields are required for the command to be accepted:

| Field | Value | Why |
|---|---|---|
| `LobbyID` | `lob.ID` (from the `Lobby` entity) | Tells the plugin which pending lobby this assignment is for. |
| `RequestID` | `lob.Session.PendingRequestID` (echoed verbatim) | Plugin rejects stale or mismatched assignments. Never generate your own — always read it from the lobby. |
| `GameWorld` | `cardinal.OtherWorld` with at minimum a non-empty `ShardID` | The full address of the game shard for this session. Empty `ShardID` = failure (regardless of `Reason`); `Reason` is only used as the failure message. |

### Example: pool-based orchestrator (probe + claim)

A realistic deployment with multiple game shards needs dynamic
assignment. The shape below probes a pool of game shards, claims the
first idle one, and releases claims when the session ends. This is a
pattern only — adapt it to your own shard naming, pool sizing, and
failure handling.

**Components** (two, both orchestrator-owned):

```go
type ShardClaim struct {
    LobbyID   string  // which lobby owns the claim
    ShardID   string  // which game shard is bound
    ClaimedAt int64   // unix seconds
}
func (ShardClaim) Name() string { return "shard_claim" }

type PendingProbe struct {
    LobbyID   string  // lobby we've already fanned out probes for
    StartedAt int64
}
func (PendingProbe) Name() string { return "pending_probe" }
```

**Custom commands** for the game-shard round trip:

```go
// Lobby → game shard: "are you idle?"
type CheckAvailabilityCommand struct {
    LobbyID       string
    SendbackWorld cardinal.OtherWorld // where to reply
}
func (CheckAvailabilityCommand) Name() string { return "check_availability" }

// Game shard → lobby: "here's my state"
type AvailabilityResponseCommand struct {
    LobbyID   string
    ShardID   string
    Idle      bool
    SessionID string  // empty if Idle
    StartedAt int64
}
func (AvailabilityResponseCommand) Name() string { return "availability_response" }
```

**Three systems working together:**

1. `ProbeOnSessionAwaitingAllocationSystem` — scans lobbies; for each
   in `awaiting_allocation` with no `PendingProbe` yet, creates a
   `PendingProbe` entity and fans out `CheckAvailabilityCommand` to
   every shard in the pool (e.g., `game-shard-1` .. `game-shard-N`).

2. `HandleAvailabilityResponseSystem` — consumes
   `AvailabilityResponseCommand`. For each `Idle=true` response whose
   lobby is still pending and whose shard isn't already claimed: create
   a `ShardClaim` entity, destroy the `PendingProbe`, send
   `lobby.AssignShardCommand{LobbyID, RequestID, GameWorld}` to the lobby
   shard itself.

3. `ReleaseOnSessionEndSystem` — scans the lobby table each tick;
   drops `ShardClaim` entities whose lobby is no longer `in_session`
   (session ended, cancelled, or lobby destroyed) and drops
   `PendingProbe` entities whose lobby left `awaiting_allocation`.

**Game-shard side:** a single responder system handles
`CheckAvailabilityCommand` and replies with
`AvailabilityResponseCommand` to `SendbackWorld`, reading idle state
from whatever singleton the game uses to track its active session.

**Why it's this many pieces:** Cardinal's only inter-shard primitive is
async `SendCommand`. Asking a shard "are you idle?" therefore requires
a command out, a command back, and state to correlate them across
ticks. Simpler orchestrators (static, round-robin) skip the probe
entirely — see the minimal example above.

### Orchestrator contract

Send `AssignShardCommand` to the lobby shard's own address with:
- `LobbyID` — the lobby awaiting allocation
- `RequestID` — echoed from `lob.Session.PendingRequestID` (the plugin
  rejects mismatches to guard against stale / duplicate commands)
- `GameWorld` — the chosen game shard's full address, or empty
  `ShardID` (+ `Reason`) to fail the start

The plugin's handler validates the `RequestID`, checks
`cmd.Persona` against `AssignmentAuthority` if configured, and rejects
anything that doesn't match. On success it writes `GameWorld` onto
`lobby.GameWorld`, transitions to `in_session`, dispatches
`NotifySessionStartCommand`, and emits the final `StartSessionResult`.

### Timing contract

Because assignment is asynchronous, `StartSessionResult` is emitted
several ticks after the `StartSessionCommand` arrives (orchestrator
round-trip + decision). Clients should not set short timeouts on
`StartSessionResult` — treat session start as a matchmaking-style wait.

## Key Behaviors

- **Invite code persists** on leader transfer/leave (no auto-regeneration)
- **Leader-only actions**: kick, transfer, start session, regenerate code
- **Ready check**: All players (including leader) must be ready to start
- **Session lock**: No join/team-change during `in_session` (leave is always allowed)
- **Auto-reset**: After session ends, all players reset to not-ready
- **Auto-delete**: Lobby deleted when last player leaves
- **Per-lobby game shard**: Each lobby specifies its target game shard via `GameWorld`

## Heartbeat System

The lobby package includes a heartbeat mechanism to detect and remove disconnected players. It uses a **deadline-based (lease) approach** where clients must continuously renew their presence.

### How It Works

1. **Clients send `HeartbeatCommand`** periodically (e.g., every `HeartbeatTimeout/3` seconds)
2. **Server stores a deadline** for each player: `deadline = now + HeartbeatTimeout`
3. **On each heartbeat**, the deadline is extended: `deadline = now + HeartbeatTimeout`
4. **Server removes players** when `now >= deadline` (player failed to renew their lease)
5. **Events emitted**: `PlayerTimedOutEvent` when a player times out

### Deadline Approach

When a player joins or sends a heartbeat:
```
deadline = currentTime + HeartbeatTimeout
```

When checking for timeouts (every tick):
```
if currentTime >= deadline {
    // Player timed out - remove from lobby
}
```

This approach is simpler than tracking "last heartbeat time" because:
- No subtraction needed at check time (`now >= deadline` vs `now - lastHeartbeat > timeout`)
- The deadline is the actual removal time, making debugging easier

### Configuration

```go
cardinal.RegisterPlugin(world, lobby.NewPlugin(lobby.Config{
    LobbyWorld:       cardinal.OtherWorld{...},
    HeartbeatTimeout: 30, // Remove player after 30 seconds without heartbeat
}))
```

### Client Implementation

Clients should:
1. Start a heartbeat timer **immediately after creating or joining a lobby**
2. Send `HeartbeatCommand` at the configured interval
3. Stop the timer after leaving the lobby

```go
// Client-side pseudocode
ticker := time.NewTicker(10 * time.Second)
go func() {
    for range ticker.C {
        sendCommand(lobby.HeartbeatCommand{})
    }
}()
```

**Important**: Start heartbeats immediately after joining. The timeout clock starts the moment a player joins, not when they send their first heartbeat.

### Timeout Behavior

When a player joins a lobby, their deadline is set to `now + timeout`. This means:
- Players must send heartbeats before their deadline expires or they are removed
- This prevents "zombie" players who join but never send heartbeats from occupying slots indefinitely

When a player times out:
- Player is removed from the lobby
- `PlayerTimedOutEvent` is emitted
- If player was leader, leadership auto-transfers to another player
- If lobby becomes empty, it is deleted
