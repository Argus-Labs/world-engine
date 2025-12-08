# Lobby Package

A party and lobby management system for Cardinal worlds, handling pre-game coordination and game lifecycle.

## Architecture Overview

This package implements an ECS-based lobby system with party management, ready-up coordination, and cross-shard game communication.

```text
World
├── PartyIndexComponent (singleton)
│   ├── PartyIDToEntity (map[string]uint32)
│   ├── PlayerToParty (map[string]string)
│   └── LobbyToParties (map[string][]string)
│
├── LobbyIndexComponent (singleton)
│   ├── MatchIDToEntity (map[string]uint32)
│   ├── ActiveLobbies ([]string)
│   └── InGameLobbies ([]string)
│
├── ConfigComponent (singleton)
│   ├── DefaultMaxPartySize
│   ├── HeartbeatTimeoutSeconds
│   └── GameShardID
│
├── Parties ([]PartyComponent)
│   ├── ID, LeaderID, Members
│   ├── IsOpen, MaxSize
│   └── LobbyID, IsReady
│
├── Lobbies ([]LobbyComponent)
│   ├── MatchID, HostPartyID, Parties
│   ├── Teams []LobbyTeam
│   ├── State (waiting/ready/in_game/ended)
│   └── Config, Timestamps
│
└── Systems
    ├── InitSystem (Init hook) - creates singletons
    ├── PartySystem (Update hook) - party commands
    └── LobbySystem (Update hook) - lobby commands, game lifecycle
```

### Core Components

1. **PartyComponent**: Group of players who queue and play together (never split)
2. **LobbyComponent**: Pre-game room with state machine (waiting → ready → in_game → ended)
3. **PartyIndexComponent**: Singleton for O(1) party lookups by ID/player/lobby
4. **LobbyIndexComponent**: Singleton for O(1) lobby lookups by match ID

### Key Design Features

1. **State Machine**: Lobbies transition through waiting → ready → in_game → ended
2. **Cross-Shard Support**: Receives matches from matchmaking, notifies game shard on start
3. **Heartbeat Timeout**: Stale lobbies are cleaned up automatically
4. **Extensible**: Users can add custom systems alongside lobby

## Installation

```bash
go get github.com/argus-labs/world-engine/pkg/lobby
```

## Usage

```go
package main

import (
    "github.com/argus-labs/world-engine/pkg/cardinal"
    "github.com/argus-labs/world-engine/pkg/lobby"
)

func main() {
    world, _ := cardinal.NewWorld(cardinal.WorldOptions{
        TickRate: 10,
    })

    lobby.Register(world, lobby.Config{
        DefaultMaxPartySize:     4,
        HeartbeatTimeoutSeconds: 60,
        GameShardID:             "game-1", // empty for same-shard
    })

    world.StartGame()
}
```

## API Reference

### Party Commands

| Command | Description |
|---------|-------------|
| `lobby_create_party` | Create a new party |
| `lobby_join_party` | Join existing party |
| `lobby_leave_party` | Leave current party |
| `lobby_set_party_open` | Toggle open/closed |
| `lobby_promote_leader` | Transfer leadership |
| `lobby_kick_from_party` | Kick member (leader only) |

### Lobby Commands

| Command | Description |
|---------|-------------|
| `lobby_create` | Create a new lobby |
| `lobby_join` | Join existing lobby |
| `lobby_leave` | Leave current lobby |
| `lobby_set_ready` | Set ready status |
| `lobby_start_game` | Start game (host only) |
| `lobby_end_game` | End game |
| `lobby_heartbeat` | Keep lobby alive during game |

## Configuration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `DefaultMaxPartySize` | int | 4 | Maximum players per party |
| `HeartbeatTimeoutSeconds` | int64 | 60 | Lobby timeout without heartbeat |
| `MatchmakingShardID` | string | "" | Source matchmaking shard (empty for same-shard) |
| `GameShardID` | string | "" | Target game shard (empty for same-shard) |

## Contributing

See [CONTRIBUTING.md](../../CONTRIBUTING.md) for development setup and guidelines.

## License

See [LICENSE](../../LICENSE) for details.
