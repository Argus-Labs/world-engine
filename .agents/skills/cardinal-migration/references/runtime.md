# Runtime, toolchain and operations

## Versions

- world-engine `v0.17.1`. It has the same Cardinal API and snapshot format as v0.17.0,
  and adds World CLI to the module.
- Go `1.27.1`. The API uses generic methods (`w.Commands[T]()`). Set `go 1.27.1` in
  go.mod; `go get` raises it for you. Builders pinned to an older Go image fail.
- World CLI ships in the world-engine module. Install the global command with
  `go install github.com/argus-labs/world-engine/cli/cmd/world@latest`. After `go get`,
  pin it in the project:

  ```sh
  go mod edit -tool=github.com/argus-labs/world-engine/cli/cmd/world
  go mod tidy
  ```

  Inside the project, `world` then runs the version go.mod pins. `world --version` there
  prints `v0.17.1`. Add the tool only after `go get`: the path does not exist in v0.16.x.
  The tool adds World CLI's dependencies to go.mod (a template game went from 91 to 254
  lines). The shard image still builds only the shard.
- The `install.world.dev` scripts and `world update` install world-cli 2.5.1, the last
  standalone release. It generates the same code for v0.17.1, but its `world mcp`
  `get_state` cannot read v0.17 state (Tooling that reads world state).

## Generated wire code

Every component, command, event and system event must implement `SizeWire() int` and
`AppendWire([]byte) []byte`. Generated code from world-cli before 2.4.9 does not, and fails
with `X does not satisfy ... (missing method AppendWire)`.

World CLI (2.5 and later) discovers wire types only from `w.RegisterCommand/RegisterEvent/
RegisterComponent/RegisterSystemEvent[T]()` calls, and from `w.SendToShard(to, Cmd{...})`
for command types inside the directory being generated. It no longer understands the
v0.16 fields (`WithCommand[T]`, `Ref[T]`, ...). So:

1. Generate after systems, entities and plugins are converted (workflow step 7). Discovery
   hides `wire.gen.go` and tolerates the missing wire methods, but every other package must
   type-check. Before that it stops with `DISCOVERY FAILED — the backend has type errors
   outside the generated wire layer`. (On untouched code, before `go get`, it runs: use
   that to size the work.)
2. Delete the stale `wire.gen.go` files the audit lists first. Once a field changes shape
   they break the build (`cannot range over c.Rows (variable of struct type
   immutable.Slice[...])`), and generation replaces them anyway.
3. Run the command the project used before (README, Makefile, CI). If nothing records it,
   match the existing `gen/` location: `world sdk generate shards/<id> --go-out
   shards/<id>/gen`. When shards share one `--go-out` (for example `./gen`), generate from
   the repo root in one run. A per-shard run into a shared directory deletes the other
   shards' output. Repo-wide generation is all-or-nothing: one violation in any shard
   blocks every shard.
4. Register the declared type, not an alias (`type Config = other.Config`). World CLI
   reports a registered alias as `undeclared`.
5. `world sdk generate --no-emit` prints discovery findings and exits non-zero if anything
   blocks.
   - `undeclared`: a type reaches a cardinal generic (`w.Commands[T]()`, `e.Get[T]()`)
     without a Register* call. Add the registration.
   - `ORPHANS` (exit 0, but treat as a failure): a type has a `Name()` but no Register*
     call. A real generate skips it. A missing `RegisterEvent` shows up only here, not
     as `undeclared`. If the game uses the type, add the registration. Expected orphans:
     types v0.16 never registered either (an unused template command, unused tags), data
     plugin kinds (`data.Register[T]` types), and look-alike packages' types.
6. `gen/**/*.pb.go` and client SDK output should not change. If they do, the proto schema
   changed; ship the new client SDK with the shard. Games scaffolded from a world-cli
   template may see generated files move and proto package names change on the first
   regenerate. The wire bytes are the same, but client SDKs still need regenerating.

World CLI refuses `[]T`, `map`, pointer and interface fields in wire types, server-only
components included. Recipes are in older-versions.md (From v0.16.4).

## Snapshots

The snapshot format changed in v0.16.9 without a version bump. Tested in a local cluster
with v0.16.7 and v0.16.9 games:

| Snapshot written by | Read by v0.17              | v0.17 snapshot read by the old build |
| ------------------- | -------------------------- | ------------------------------------ |
| v0.16.8 or earlier  | fails, shard restart-loops | wrong world, no error                |
| v0.16.9             | restores                   | restores                             |

Details for v0.16.8 or earlier:

- New build, old snapshot: `failed to restore state from snapshot: refusing to restore
  snapshot: failed to unmarshal snapshot: proto: cannot parse invalid wire-format data`,
  logged as `failed running world`. `StartGame` returns and the process exits with code 0,
  so Kubernetes shows `Completed` and restarts it forever. It does not overwrite the
  snapshot, so every restart fails the same way. Look for this line in the shard logs,
  not for a crash.
- Old build, new snapshot: restores with no error and a wrong world. It keeps the tick
  height and loses the entities (a v0.16.7 build came up with 0 of 11), or shows phantom
  ones.

What to tell the user:

- From v0.16.8 or earlier, delete each shard's snapshot at deploy. JetStream: object
  `snapshot` in bucket `<org>_<project>_<shardID>_snapshot`. S3: key
  `<org>/<project>/<shardID>/snapshot` in `CARDINAL_S3_BUCKET`. Or point the shard at a
  fresh location.
- Local: `world purge` deletes the whole local k3d cluster, snapshots included. Then
  `world start` again.
- Rolling back to v0.16.8 or earlier requires deleting the new snapshots first.
- A shard with no `SnapshotStorageType` in `main.go` and no
  `CARDINAL_SNAPSHOT_STORAGE_TYPE` defaults to NOP and never persisted state, so there is
  nothing to wipe. The multi-shard template is one. Check before telling the user.
- A snapshot that holds a component the build no longer registers also fails to restore:
  `snapshot entity N holds component "x", which this build does not register`.

Do not delete remote snapshots yourself.

## Telemetry

Every tick is now a root span with one child span per system, sampled at
`OTEL_TRACE_SAMPLE_RATE` (default 1.0, always sample). Set it explicitly in production.

| Env var                                              | Change                                                                                                                             |
| ---------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| `OTEL_EXPORTER_OTLP_ENDPOINT`                        | Default is now `groundcover-sensor.groundcover.svc.cluster.local:4317` when unset. Empty disables tracing. Accepts `https://` URLs |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`                 | New. Wins over the generic var                                                                                                     |
| `OTEL_EXPORTER_OTLP_INSECURE`, `..._TRACES_INSECURE` | New. Apply to bare host:port                                                                                                       |

Under `world start`, shards log `traces export: exporter export timeout ... dial tcp
...:4317: i/o timeout` about every 15 seconds. The local operator points them at
`cardinal-operator:4317` and serves OTLP on that port, but the operator Service that World
CLI installs only exposes 8090. It is harmless, and `enable_otel` in world.toml does not
change it.

In v0.17.1, `CARDINAL_*` variables and `world.toml` are unchanged. A release with
`cardinal.NewTestWorld` removes `CARDINAL_PPROF` (bootstrap.md, Testing one system). Code
that imported `pkg/telemetry` directly: `Telemetry.Tracer` is gone. Use
`pkg/telemetry/trace.New`.

## Tooling that reads world state

The debug `GetState` response now carries a component name table plus entities instead of
archetypes and columns. Tools built against v0.16.8 or earlier show an empty world against a
v0.17 shard (`entity_count 0`): world-cli 2.5.1's `world mcp` `get_state`, and Cardinal
Editor builds from before v0.17. The project-pinned World CLI reads it: run `world mcp`
inside the project and call `get_state` with the shard ID. It returns every entity with
decoded components.

Game tools that read GetState walk the name table:

```go
ws := resp.GetSnapshot().GetWorldState()
for _, e := range ws.GetEntities() {
	for i, idx := range e.GetComponents() {
		name := ws.GetComponents()[idx] // component Name()
		payload := e.GetPayloads()[i]   // AppendWire bytes; decode with T{}.UnmarshalWire
		_, _ = name, payload
	}
}
```
