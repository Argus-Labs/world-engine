# Matchmaking Package

A ticket-based matchmaking system for Cardinal worlds, supporting filters, team composition, and backfill.

## Architecture Overview

This package implements an ECS-based matchmaking system with singleton indexes for O(1) lookups and profile-driven matching.

```text
World
├── IndexComponent (singleton)
│   ├── TicketsByProfile (map[string][]uint32)
│   ├── TicketsByParty (map[string]uint32)
│   └── PlayerToParty (map[string]string)
│
├── ConfigComponent (singleton)
│   ├── DefaultTTLSeconds
│   ├── BackfillTTLSeconds
│   └── LobbyShardID
│
├── Tickets ([]TicketComponent)
│   ├── TicketID, PartyID, MatchProfileName
│   ├── Players []PlayerInfo
│   └── SearchFields (StringArgs, DoubleArgs, Tags)
│
├── Profiles ([]ProfileComponent)
│   ├── ProfileName
│   ├── Pools []PoolConfig (filters, min/max)
│   └── Teams []TeamConfig (pools, min/max)
│
└── Systems
    ├── InitSystem (Init hook) - creates singletons
    └── MatchmakingSystem (Update hook) - processes commands, runs matching
```

### Core Components

1. **TicketComponent**: Represents players queuing together with search criteria
2. **ProfileComponent**: Defines matching rules (pools, filters, team composition)
3. **IndexComponent**: Singleton providing O(1) ticket lookups by profile/party/player
4. **ConfigComponent**: Singleton storing TTL and cross-shard settings

### Key Design Features

1. **Profile-Driven Matching**: Tickets match against configured profiles with pool filters
2. **Cross-Shard Support**: Sends matches to lobby via command (cross-shard) or event (same-shard)
3. **Backfill Support**: Fill empty slots in ongoing matches
4. **Extensible**: Users can add custom systems alongside matchmaking

## Installation

```bash
go get github.com/argus-labs/world-engine/pkg/matchmaking
```

## Usage

```go
package main

import (
    "github.com/argus-labs/world-engine/pkg/cardinal"
    "github.com/argus-labs/world-engine/pkg/matchmaking"
)

func main() {
    world, _ := cardinal.NewWorld(cardinal.WorldOptions{
        TickRate: 10,
    })

    matchmaking.Register(world, matchmaking.Config{
        DefaultTTLSeconds:  300,
        BackfillTTLSeconds: 60,
        LobbyShardID:       "lobby-1", // empty for same-shard
    })

    world.StartGame()
}
```

## API Reference

### Commands

| Command | Description |
|---------|-------------|
| `matchmaking_create_ticket` | Create a matchmaking ticket |
| `matchmaking_cancel_ticket` | Cancel a ticket |
| `matchmaking_create_backfill` | Request backfill for ongoing match |
| `matchmaking_cancel_backfill` | Cancel backfill request |

### Events

| Event | Description |
|-------|-------------|
| `matchmaking_ticket_created` | Ticket created successfully |
| `matchmaking_ticket_cancelled` | Ticket was cancelled |
| `matchmaking_ticket_error` | Ticket operation failed |
| `matchmaking_match_found` | Match found, sent to players |
| `matchmaking_backfill_match` | Backfill tickets assigned |

## Configuration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `DefaultTTLSeconds` | int64 | 300 | Ticket time-to-live in seconds |
| `BackfillTTLSeconds` | int64 | 60 | Backfill request TTL in seconds |
| `LobbyShardID` | string | "" | Target lobby shard (empty for same-shard) |

## Match Profiles

Match profiles define how tickets are grouped into matches. Create profiles using a custom init system:

```go
// In your custom init system
type ProfileLoaderState struct {
    cardinal.BaseSystemState
    Profiles cardinal.Contains[struct {
        Profile cardinal.Ref[matchmaking.ProfileComponent]
    }]
}

func ProfileLoaderSystem(state *ProfileLoaderState) error {
    // Simple 1v1
    _, e := state.Profiles.Create()
    e.Profile.Set(matchmaking.ProfileComponent{
        ProfileName: "1v1-ranked",
        Pools: []matchmaking.PoolConfig{
            {Name: "default", Filters: map[string]string{"region": "NA"}, MinPlayers: 1, MaxPlayers: 1},
        },
        Teams: []matchmaking.TeamConfig{
            {Name: "team_1", Pools: []string{"default"}, MinPlayers: 1, MaxPlayers: 1},
            {Name: "team_2", Pools: []string{"default"}, MinPlayers: 1, MaxPlayers: 1},
        },
        MinPlayers: 2,
        MaxPlayers: 2,
    })
    return nil
}
```

### Profile Examples

**1v1 Ranked** - Two players, single pool:
```go
ProfileComponent{
    ProfileName: "1v1-ranked",
    Pools: []PoolConfig{
        {Name: "default", Filters: map[string]string{"region": "NA"}},
    },
    Teams: []TeamConfig{
        {Name: "team_1", Pools: []string{"default"}, MinPlayers: 1, MaxPlayers: 1},
        {Name: "team_2", Pools: []string{"default"}, MinPlayers: 1, MaxPlayers: 1},
    },
    MinPlayers: 2, MaxPlayers: 2,
}
```

**5v5 with Roles** - Role-based team composition (1 tank, 3 dps, 1 support per team):
```go
ProfileComponent{
    ProfileName: "5v5-roles",
    Pools: []PoolConfig{
        {Name: "tank", Filters: map[string]string{"role": "tank"}},
        {Name: "dps", Filters: map[string]string{"role": "dps"}},
        {Name: "support", Filters: map[string]string{"role": "support"}},
    },
    Teams: []TeamConfig{
        {Name: "team_1", Pools: []string{"tank", "dps", "dps", "dps", "support"}, MinPlayers: 5, MaxPlayers: 5},
        {Name: "team_2", Pools: []string{"tank", "dps", "dps", "dps", "support"}, MinPlayers: 5, MaxPlayers: 5},
    },
    MinPlayers: 10, MaxPlayers: 10,
}
```

**Battle Royale** - 10 solo players:
```go
ProfileComponent{
    ProfileName: "battle-royale",
    Pools: []PoolConfig{
        {Name: "default", Filters: map[string]string{"region": "NA"}},
    },
    Teams: []TeamConfig{
        {Name: "player_1", Pools: []string{"default"}, MinPlayers: 1, MaxPlayers: 1},
        {Name: "player_2", Pools: []string{"default"}, MinPlayers: 1, MaxPlayers: 1},
        // ... repeat for all 10 players
    },
    MinPlayers: 10, MaxPlayers: 10,
}
```

**4-Team Squads** - Four teams of 3 players each:
```go
ProfileComponent{
    ProfileName: "4-team-squads",
    Pools: []PoolConfig{
        {Name: "default", Filters: map[string]string{"region": "NA"}},
    },
    Teams: []TeamConfig{
        {Name: "team_1", Pools: []string{"default"}, MinPlayers: 2, MaxPlayers: 3},
        {Name: "team_2", Pools: []string{"default"}, MinPlayers: 2, MaxPlayers: 3},
        {Name: "team_3", Pools: []string{"default"}, MinPlayers: 2, MaxPlayers: 3},
        {Name: "team_4", Pools: []string{"default"}, MinPlayers: 2, MaxPlayers: 3},
    },
    MinPlayers: 8, MaxPlayers: 12,
}
```

## Contributing

See [CONTRIBUTING.md](../../CONTRIBUTING.md) for development setup and guidelines.

## License

See [LICENSE](../../LICENSE) for details.
