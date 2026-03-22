# methane

[![Go Reference](https://pkg.go.dev/badge/github.com/opd-ai/methane.svg)](https://pkg.go.dev/github.com/opd-ai/methane)

**methane** is a decentralized game matchmaking service built on top of the [Tox P2P protocol](https://tox.chat) via [`github.com/opd-ai/toxcore`](https://github.com/opd-ai/toxcore). It provides competitive matchmaking features usable as an **importable Go library** for in-game integration and as a **standalone CLI launcher** for third-party games.

## Features

- **Lobby System** — Create and join game lobbies backed by Tox conferences. Public lobbies are advertised to friends; private lobbies use direct friend invitations.
- **Matchmaking Queue** — Players enqueue with a game mode and region preference. The Glicko-2 rating-based matching algorithm groups players with progressively widening windows to prevent starvation.
- **Glicko-2 Rating** — Deterministic, pure-function implementation of the Glicko-2 skill rating algorithm with injectable `TimeProvider` for testing.
- **Game Sessions** — Full lifecycle: MatchFormed → ReadyCheck → Launching → InProgress → Completed. Ready-check acknowledgements collected via friend messages.
- **Game Launcher** — Spawn any game executable with connection parameters passed as CLI arguments and/or environment variables.
- **Pure Go, zero CGo** — Follows all toxcore conventions: structured logrus logging, `sync.RWMutex` thread safety, injectable time providers, `//export` annotations.

## Installation

```sh
go get github.com/opd-ai/methane
```

## Library Usage

```go
package main

import (
    "context"
    "log"

    "github.com/opd-ai/methane"
    "github.com/opd-ai/toxcore"
)

func main() {
    // Create a Tox instance
    opts := toxcore.NewOptions()
    opts.UDPEnabled = true
    tox, err := toxcore.New(opts)
    if err != nil {
        log.Fatal(err)
    }
    defer tox.Kill()

    // Bootstrap to the Tox network
    _ = tox.Bootstrap("node.tox.biribiri.org", 33445,
        "F404ABAA1C99A9D37D61AB54898F56793E1DEF8BD46B1038B9D822E8460FAB67")

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Create and start the matchmaking service
    svc := methane.NewMatchmakingService(tox)
    svc.OnMatchFound(func(evt *methane.MatchFoundEvent) {
        log.Printf("Match found! Session: %s, Players: %d", evt.SessionID, len(evt.Players))
    })
    if err := svc.Start(ctx); err != nil {
        log.Fatal(err)
    }
    defer svc.Stop()

    // Create a public lobby
    lobby, err := svc.CreateLobby(methane.LobbyConfig{
        GameName:   "MyGame",
        MapName:    "dust2",
        GameMode:   methane.ModeFFA,
        MaxPlayers: 4,
        Region:     methane.RegionNA,
    })
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Lobby %d created", lobby.ID)

    // Or enter the matchmaking queue
    if err := svc.EnqueueForMatch(methane.ModeFFA, methane.RegionNA); err != nil {
        log.Fatal(err)
    }

    select {} // block until match found
}
```

## CLI Usage

Build the CLI:

```sh
go build -o methane ./cmd/methane
```

### Subcommands

| Command | Description |
|---|---|
| `methane host` | Create a new game lobby |
| `methane browse` | List lobbies from all friends |
| `methane join <lobby-id>` | Join a lobby by ID |
| `methane queue <game>` | Enter the matchmaking queue for a game |
| `methane launch` | Start the configured game executable when matched |

### Common Flags

| Flag | Default | Description |
|---|---|---|
| `--save-file` | `methane.tox` | Path to Tox save file |
| `--name` | `methane-player` | Display name on the Tox network |
| `--bootstrap` | _(empty)_ | Bootstrap node in `host:port:pubkey` format |

### Examples

```sh
# Host a lobby
methane host --game "Quake" --mode ffa --max-players 4 --region na \
    --name "HostPlayer" --bootstrap "node.tox.biribiri.org:33445:F404A..."

# Browse lobbies (asks all friends)
methane browse --save-file player.save

# Join a lobby by ID
methane join 12345 --save-file player.save

# Enter matchmaking queue for a game
methane queue quake --save-file player.save

# Launch game when matched, passing connection info to executable
methane launch --exec /usr/games/quake --save-file player.save
```

## Package Structure

| File | Purpose |
|---|---|
| `matchmaker.go` | `MatchmakingService` — top-level type, lifecycle, Tox wiring |
| `lobby.go` | `Lobby` — conference-backed lobby with player roster |
| `queue.go` | `MatchmakingQueue` — Glicko-2 + preference-based matching |
| `player.go` | `Player` — identity, stats, Glicko-2 rating fields |
| `session.go` | `GameSession` — state machine: ready-check → launch → result |
| `rating.go` | Glicko-2 algorithm — pure functions, deterministic |
| `protocol.go` | JSON message envelope, encode/decode helpers |
| `types.go` | Shared enums, constants, `TimeProvider` interface |
| `launcher.go` | `GameLauncher` — spawn and monitor game process |
| `doc.go` | Package-level godoc |

## Protocol

All peer-to-peer communication uses JSON-encoded `Envelope` messages transmitted as Tox friend messages or conference messages:

```json
{ "type": "lobby_ad", "payload": { "lobby_id": 1, "game_name": "Quake", ... } }
```

Message types: `lobby_ad`, `match_found`, `ready_check`, `ready_response`, `game_launch_info`, `game_result`, `lobby_list`.

## Rating System

Methane uses the **Glicko-2** skill rating algorithm. New players start at rating **1500** with deviation **350** and volatility **0.06**. Ratings are updated after each session using the `CalculateNewRating` function in `rating.go`.

## License

See [LICENSE](LICENSE).
