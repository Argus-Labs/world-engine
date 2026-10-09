---
name: world-local-dev
description: Run, reload, inspect and reset this World Engine game on the developer's Docker with the `world` CLI. Use for "start the game", "my shard won't come up", "show me the logs", "reset the world state", "stop everything".
---

# World Engine local development

`world` runs the game as Docker containers: one network per project, one NATS and one
Postgres per project, and one container per shard instance. A reverse proxy inside the
`world start` process serves every shard on `localhost:8080`. Nothing runs outside Docker
and that process.

Prefer the project's pinned CLI: run `go tool world …` in the project directory (falls back to
`world` if the project does not declare the tool).

## Commands

| Goal | Command | Notes |
|---|---|---|
| Check prerequisites | `world doctor` | Git, Go, Docker daemon |
| Build and run everything | `world start` | Starts NATS + Postgres, builds `<project>-<shard>-shard` images, one container per `[[shards]]` instance, then opens the log picker. **Stays attached: Ctrl+C stops every container (data kept)** |
| Rebuild and roll shards | `world reload [instance…]` | Rebuilds the image and recreates the containers; state kept. Instances are `gameplay`, `gameplay-2`, … |
| Rebuild from a clean world | `world reload --purge [instance…]` | Removes the pool's containers, wipes the named instances' JetStream state, recreates on the new image |
| Logs | `world logs` | Picker over shards, NATS, project DB, `[[services]]`. `--context <ctx> --namespace <ns>` tails a Kubernetes environment instead |
| Stop, keep data | `world stop` | Stops all containers; volumes stay |
| Delete the world | `world purge` | Removes containers, volumes and the network. `--image` also prunes the built images |

Everything is also visible with plain Docker: `docker ps -a --filter label=world.argus.gg/project=<project>`,
`docker logs <project>-<instance>-shard`, `docker volume ls --filter name=<project>-`.

## Where things are

- Shard API: `http://localhost:8080/<organization>/<project>/<instance>` (the edge proxy in
  `world start`). Instances are `<shard>`, `<shard>-2`, … for `pool_size > 1`. The same path
  works against a hosted environment.
- Each shard is also on `127.0.0.1:8081`, `8082`, … in `world.toml` order (direct, no proxy).
- NATS: `nats://127.0.0.1:4222`. Project DB: `localhost:5432`, user/password `postgres`,
  database `<project>`. Created for every project unless a `[[services]]` entry sets
  `config_db = true` and brings its own.
- The DebugService (pause, step, reset, state) is on the shard URL; `world debug` and the MCP
  tools use it.

## Signing in

Shards authenticate players the way `world.toml` says. Without an `[auth]` section
they run dev auth: the dev identity header names the player, so no auth service is
involved and each identity is its own player.

```toml
[auth]
mode = "argus"              # default is "dev"
url = "https://api.argus.dev"  # required for argus; ignored for dev
```

`argus` makes a local world behave like a hosted one: shards validate real Argus Auth
tokens, so the game client signs in with its normal account. The shards must reach
`url` at startup, and `send_command` then needs a token — `WORLD_ARGUS_TOKEN=<token>`
makes the MCP tools send it, and only ever to a shard on this machine. `world debug`,
`introspect` and `get_state` use the DebugService, which is unauthenticated either way.

## Rules for agents

- Leaving `world start` (Ctrl+C or quitting the picker) stops the world. Keep it running in
  its own terminal while testing; use `world logs` and `world reload` from another one.
- Do not `docker run` or `docker rm` shard containers by hand next to a running `world start`;
  change `world.toml` and run `world reload`.
- `world purge` deletes player and world state for this project. Ask before running it unless
  the user said "reset" or "start over".
- Ports 8080, 4222 and 5432 are per machine, so one project runs at a time.

## When something fails

| Message | Cause | Do |
|---|---|---|
| `edge cannot listen on 127.0.0.1:8080` | Another process owns the port (an old `world start`, a k3d cluster, another proxy) | `lsof -i :8080`; stop the other listener |
| `port is already allocated` on 4222 / 5432 | A local NATS or Postgres, or another project's containers | `lsof -i :5432`; stop it, or `world stop` the other project |
| `timeout waiting for <project>-nats` / `-db to become ready` | Container started but never healthy | `docker logs` the container the message names, not the other one; if its volume is corrupt, `world purge` — ask first, it wipes all state |
| Shard container `exited` right after start | Game code panics, or DB unreachable | `world logs` → the shard; `DB_DSN` host is `<project>-db`, or the `config_db` service's container if `world.toml` declares one |
| An auth-header error from a client | The world runs dev auth and the client sent an Argus token | Send the dev header, or set `[auth] mode = "argus"` in `world.toml` |
| `failed to fetch JWKS` on startup | `[auth].url` is wrong or unreachable from the container | `curl <url>/auth/jwks`; it must answer 200 |
| `build constraints exclude all Go files … cgo` | The shard needs cgo; `world start` builds with `CGO_ENABLED=0` | Not supported by `world start` yet |
| `world is not running` from `world logs` / `world reload` | The project's NATS container is not running, so no world was started here | `world start` first |
