# sdkgen (maintainer notes)

Audience: whoever **maintains or extends** this generator. Developer-facing usage is
in `world sdk generate --help`; the type/authoring rules are enforced by the lint (it
names the fix per violation).

## What it does

Reads a World Engine backend's Go wire types — commands, events, components, and
system events — and produces typed, reflection-free serialization for Go and C#:

```
                        .proto --(protoc/buf in Docker)--> proto structs (Go) + C# classes (gen/)
Go wire structs  --→  /
  (discover roles,    \
   classify, lint)     wire.gen.go (in-place) --→ ToProto/FromProto (struct ⇄ proto message)
                                                  + SizeWire/AppendWire (every type)
                                                  + MarshalWire/UnmarshalWire (top-level types)
```

- **C#:** the protoc-generated class (reflection-free, IL2CPP-safe) is used directly.
- **Go:** the hand-written struct stays as-is (only its `Name()` method). Generated
  methods convert it to/from the protoc-generated proto message and (de)serialize via
  protobuf — no runtime reflection.

## The engine contract: `schema.Serializable`

Every top-level wire type satisfies world-engine's single interface
(`pkg/cardinal/internal/schema`):

```go
type Serializable interface {
    Name() string
    MarshalWire() []byte
    UnmarshalWire([]byte) (any, error)
}
```

That is the released engine (v0.16.x). The snapshot rework (world-engine branch
`Anthony/rework-snapshot-type`) swaps `MarshalWire` for `SizeWire() int` and
`AppendWire([]byte) []byte`. The generator emits all four, so a backend builds against either.

The methods are emitted **on the type** (value receivers), so the type *is* `Serializable` —
there is **no codec struct, no registry, and no `init()`/blank-import**. `UnmarshalWire`
proto-unmarshals then `FromProto`s, returning the value as `any` (callers assert the concrete
type). A type that hasn't been generated is missing these methods, so it fails to compile — the
LSP flags it.

`MarshalWire` is **derived**: `c.AppendWire(make([]byte, 0, c.SizeWire()))`. It used to be
`proto.Marshal(c.ToProto())`, a second encoder that had to agree with the first byte for byte.
Once the rework lands it stays as a convenience and drops off the interface.

`UnmarshalWire` keeps its error: it decodes bytes from storage or the network, where
truncation and corruption are conditions rather than bugs.

The runtime dispatches by concrete type (the command queue and ECS column are generic
over `T`), never by a name lookup — so there is nothing to register.

### The direct encoders: `SizeWire` / `AppendWire`

```go
SizeWire() int             // the exact encoded size
AppendWire([]byte) []byte  // appends exactly that many bytes
```

The reworked ECS column snapshots through these: one pass sizes every component of every
entity, a second writes them all into one buffer, and neither allocates. The pair is emitted for
**every** message, nested and mirrored included, because a parent encodes a nested field through
the field's own pair. `AppendWire` never sizes: a nested message's length prefix is a one-byte
placeholder patched in after its body (`wireLenPrefix`), so the append pass is a single walk. The
bytes are the ones `proto.Marshal(c.ToProto())` produces, so `UnmarshalWire` decodes either and
the encoding rules are protobuf-go's, not ours — see the header of `emit_wire_direct.go` for the
list. Decode is unchanged.

**UTF-8.** `SizeWire` panics on a string field holding bytes that are not valid UTF-8, via
the emitted `wireStringSize` helper. proto3 forbids such a string and every decoder rejects
the payload, so writing one produces a snapshot that cannot be restored. `proto.Marshal`
made the same check, and the sizing pass runs over the whole world before a byte is
appended — so the panic fails the write instead of committing a file that only fails later,
at restore. A `bytes` field is not checked; it carries arbitrary octets by definition.

## Kinds and targets

A type's **kind** comes from the `WithX` field a system declares it through — the only
signal that a plain struct is a wire type (and one the runtime needs anyway):

| Kind | Discovered via | Targets |
|---|---|---|
| command | `RegisterCommand[T]()`, `SendToShard(T)` | Go + C# |
| event | `RegisterEvent[T]()` | Go + C# |
| component | `RegisterComponent[T]()` | Go + C# |
| system event | `RegisterSystemEvent[T]()` | Go only (in-process; the client never sees it) |

Post-codec, kind no longer changes the emitted code — every kind gets identical wire
methods. It survives only as the **target** policy (`kindTargets`). A type can play
**several roles** (e.g. an event that's also a system event): it is generated **once**,
and its `Targets` is the **union** of every role's targets (`unionTargets`) — not a
clash. `targetsFor` also filters by locality: a type defined in a dependency ships its
own Go wire code, so only its missing C# is (re)generated here.

## Pipeline (sdkgen.go)

- `Discover(dir)` — loads packages via `go/packages`, walks the full import graph (so
  types in imported packages, including plugins in *other modules*, are seen). Collects
  each `WithX` roster and enqueues every `T` under its kind; `enqueue` records the role
  **set** per type and pulls in nested field types transitively.
- `discoverGenericFieldTypes` — finds every `T` in a `cardinal.<generic>[T]` field
  (matched structurally; engine-agnostic, no world-engine import). `discoverSentCommandTypes`
  adds send-only commands (`SendCommand`/`SendToShard`), which the receive-side scan misses.
- `collectWireNames` — extracts the wire name from each type's `Name() string` (AST: the
  returned string literal; carries the version, e.g. `"move.v2"`).
- `buildMessage` — one struct → one proto message: `Kind` is a top-level role if the type
  has one (else nested), `Targets` is the union, fields numbered by declaration order.
- `reportIssues` / `checkDuplicates` — post-discovery integrity checks decided once every
  message is known (duplicate proto full name; duplicate command wire name).
- `reportOrphans` — completeness net: a **local** type with a `Name()` badge that's in no
  roster and reached as no nested field is flagged (`res.Orphans`) — it looks like a wire
  type but was never wired, so it'd be silently omitted. Surfaced in the report's ORPHANS
  section. (The roster is what *discovers* a type; the badge is what the dev *intends* — the
  diff catches a forgotten `WithX`.)

## The type map (classify — edit this to support more types)

- `classify(t)` — shape dispatch: slice → `classifySlice`, map → `classifyMap`, else
  `classifySingle`. Before any of those, world-engine's `immutable.Slice[T]` is recognised by
  origin (`engineSliceElem`) and goes to `classifyEngineSlice`, since every
  struct rule would otherwise refuse it: it has no exported field.
- `classifyEngineSlice` — `immutable.Slice[T]` → the wire shape `[]T` would have (repeated, or
  bytes for a byte element), flagged `Field.Slice`. The element goes through the slice rules, and
  `shapeCategory`/`notData` look through the Slice, so a pointer inside it is reported as a pointer.
  A Slice inside another repeated field (`Slice[Slice[T]]`, `[N]Slice[T]`) is refused like `[][]T`.
- `scalarOf` — Go scalar → proto scalar (**add a scalar here**). `uint8`→`uint32`,
  `int8/16`→`int32`, `int/uint`→64-bit.
- `classifySingle` — scalar; named struct → nested message (enqueued); `time.Time` →
  `google.protobuf.Timestamp`; `*struct`/`*scalar` via `classifyPointer` (nullable message
  / proto3 `optional`); interface → `errAny`.
- `classifySlice`/`classifyMap` — `[]byte`→bytes, `[]T`→repeated; map keys must be a
  non-float scalar, values can't be a collection.

A field that isn't cleanly representable is **refused**, and generation stops. There is one tier:
every finding blocks. The category picks which fix the report offers, and `refusedCategory` chooses
it from the type — three outcomes, asked in this order:

- **`unserializable`** — not data at all, so no encoding of any format could carry it:
  `chan`/`func`/`unsafe.Pointer`, a method interface, or a non-empty struct with no exported fields
  (nothing crosses the wire, so generating it would emit an empty message and drop the value; an
  empty `struct{}` carries nothing to lose and is allowed). `notData` walks the type, because the
  reason is often below the surface — `[]chan int` is refused for the chan, not for the slice.
- **its shape** — data reached by address, so the shape names the fix: `slice`, `map`, `pointer`,
  `interface`.
- **`unsupported-type`** — data held inline that this generator has no mapping for: `uintptr`,
  `complex64`/`complex128`. Named for who is declining rather than for protobuf's capability, since
  proto has no multi-dimensional array type either and fixed arrays are carried anyway by flattening
  them. A complex could ride as a `{re, im}` message; an address means nothing on the far side.

Plus the duplicate-name / duplicate-wire-name collisions, which are about names rather than fields.

Separately from representability, every field that DOES classify is checked for whether it can be
**stored**. A field holding a reference is reported under the Go type that causes it — `slice`,
`map`, `pointer`, `interface` — since the bytes of such a field are not the whole value and an ECS
column copy would share memory with the live world. `[N]T` is the fix and is first-class:
dimensions flatten to one `repeated` field, and `[N]byte` becomes proto `bytes`.

For a length that is genuinely unbounded the fix is world-engine's `immutable.Slice[T]`
(`pkg/immutable`). It travels exactly as `[]T` would, and it is not a finding, because nothing can
write through it: the backing array is unexported and every reader returns a copy, so a column copy
sharing memory with the live world is harmless. Its element is held to every rule a field is.

That holds only while the ELEMENT is representable. `[2][]int32` and `[4]map[string]int32` are
arrays of a collection, which protobuf cannot express any more than it can the element alone, so
they are refused — reported under the element's shape (`slice`, `map`) rather than as an array.
Fixing the element is what makes the array typed.

The test is whether a field **copies cleanly**: assigning the component gives you your own value, so
a write through one copy is never visible through another. That is narrower than "holds no pointer",
and the difference is `string`. A string header carries a pointer to heap bytes exactly as a slice
header does, and it is allowed — nothing can write through it, so no copy can affect another. Arrays
reach the same place from the other side, by copying their contents outright, and stay freely
mutable. Two mechanisms, one property:

```go
a := [3]int{1, 2, 3}; b := a; b[0] = 99 // a stays [1 2 3] — b is your own
c := []int{1, 2, 3};  d := c; d[0] = 99 // c becomes [99 2 3] — d is the same array
```

So neither immutability, boundedness, nor heap placement is the rule, and each of those admits the
wrong set: `string` is unbounded, heap-backed and pointer-carrying and passes; `[N]T` is mutable and
passes. What actually breaks is that a query hands back a copy, so within one struct `c.Name = x`
touches only the caller's copy while `c.Items[0] = x` writes into the stored component — adjacent
lines, opposite behaviour, nothing at the call site to tell them apart.

One finding per field, named after the shape. An earlier split between "proto cannot express this"
and "a column cannot hold this" reported the same field twice under two names, for one defect with
one fix.

There is no non-blocking tier. A field protobuf cannot express is refused rather than carried as
opaque JSON bytes with a warning, so a finding has one consequence and needs nothing to qualify it.
The category is decided by the TYPE, never by what `encoding/json` happens to accept: deciding it by
encodability files a `map[float64]V` under `unserializable`, when it is a map and wants a map's fix.

## Types from another module

Two cases, decided by whether that module ran the generator itself.

**It did** — its `wire.gen.go` exists, so its generated Go type does too. The field is stamped with
the owner's `go_package` and the `.proto` is written but left out of `--path`, so buf resolves it
without generating: `protoc-gen-go` emits an *import* of their package rather than a second copy of
the type. One definition, and the owner's own `ToProto` is assignable here. Every plugin type takes
this path.

**It did not** — there is nothing to import, so the type is *mirrored*: its message is emitted into
this schema and its converters as free functions in the referencing package, since Go forbids
declaring methods on a foreign type. The walk continues into its own fields to whatever depth they
go; `d.seenKind` terminates it (import cycles are irrelevant — `type A struct{ B *A }` is legal in
one package).

A type reached as a *registered* type rather than a field — `RegisterCommand[T]()` — takes neither
path: it goes to `externalTop`, where its Go comes from the owner and only the C# it does not ship
is generated here.

An **embedded** struct is not special-cased: it flows through `classify` like any other
field (its field name is the embedded type's name), so it **nests** as a sub-message —
the `ToProto`/`FromProto` composition follows the Go structure, no flattening. A cyclic
embed (valid Go via pointers) becomes a recursive proto message (dedup stops discovery
from looping).

Each violation names its fix (`categoryGuidance`), surfaced verbatim by the
lint. To add a supported type, extend `scalarOf` or `classify*`; to reject one with
guidance, return an error whose text names the fix.

## The wire layer (emit_wire.go)

`RenderGoWire` emits each package's `wire.gen.go`:

- Per message: exported `ToProto`/`FromProto` (struct ⇄ the protoc-generated proto
  message). **Value receivers, both** — `FromProto` returns a populated copy
  (`func (c X) FromProto(p) X { ...; return c }`), keeping the method set all-value (its
  hand-written `Name()` must be a value receiver under a generic value constraint, and Go/
  `recvcheck` forbid mixing value and pointer receivers). Decode is `c = c.FromProto(&p)`;
  a nested field is `c.F = c.F.FromProto(p.F)`. Exported so a type in one package can
  convert a nested type defined in another package of the same module.
- Per message: `SizeWire`/`AppendWire`, the allocation-free direct encoders
  (`emit_wire_direct.go`). Nested types get them too — their parent encodes through them.
  Mirrored types get free functions (`mirror<Alias><Name>SizeWire`/`AppendWire`). Package-level
  helpers (`sizeWireTimestamp`/`appendWireTimestamp`, `wireBool`, `wireStringSize`,
  `wireLenPrefix`) are written once per file when a field needs them, recorded like imports
  (`wireGen.helper`).
- Per **top-level** type: `MarshalWire`/`UnmarshalWire` (proto bytes). Nested types do not
  get these — they compose into their parent's message and never cross the wire standalone.
- The gen (proto) package is referenced through the alias threaded from `--go-out`, never a
  literal `"gen"`, so any output dir name works.
- A `Field.Slice` field (`immutable.Slice[T]`) reads through `All` in `ToProto` (`Clone` for
  bytes) and is rebuilt with `SliceOf` in `FromProto`. The elements collect in a per-field
  `items<Name>` slice first, because a Slice cannot be appended to in place. The file imports the
  declaring package under `Field.SliceImport`.

## protoc/buf in Docker

The `.proto` is compiled by `buf` (protoc-gen-go + C# backends) inside a pinned Docker
image, built on demand from the embedded `buf/Dockerfile` and cached by version tag — so
the build cost is one-time. Invoked by shelling out to the `docker` CLI (`os/exec`); works
with whatever that CLI targets. Two passes filter by target (see `buf.go`):

- `RunBufGo` — Go only (`buf.gen.go.yaml`), over every locally-defined type **including
  system events** (engine-internal, Go-only).
- `RunBufCSharp` — C# only (`buf.gen.cs.yaml`), over the client-facing subset (system
  events excluded) plus dependency types the client needs but that ship no C#.
- `EnsureImage` — preflights `docker` on PATH and bakes plugin versions in via build args
  (`--go-version`/`--csharp-version`).

## Storing the schema (`--proto-out`)

**Debugging only, slated for deprecation.** The emitted `.proto` is normally handed to buf and
discarded. Pass `--proto-out DIR` to also **persist** it (mirroring each file's module-relative
path), which is how you read the wire contract by hand. It is not a build input: the export holds
only this backend's own files — a dependency's schema is left out while the `import` line naming it
stays — so it does not compile standalone and cannot feed `buf breaking`. Consumers do not need it
either: a module that imports these types rebuilds their schema from the Go source in the module
cache.
Requires `--go-out` — the stored schema's `go_package` option is derived from it.

## Field numbering

Declaration order (no `pb` tags). Safe under the append-only rule: add fields at the
bottom; reorder/remove/retype = breaking = a new `vN` via `Name()`. A lock file
(pin-number-by-name, reserve deletions) is the future hardening for when shipped clients
or persisted data make version skew possible.

## Tests

`sdkgen_test.go` / `emit_wire_test.go` are self-contained: each case writes a small source
into a temp module (with a minimal `cardinal` shim), runs `Discover` / `RenderGoWire`, and
asserts inline — no committed fixtures, no golden files.

- `TestProtoTypeMapping` — the Go→proto type matrix (widening, repeated, map).
- `TestProtoNested` — nested struct, pointer, and repeated-message emission.
- `TestMultiRole` — a type wired under two roles generates once with unioned targets (not
  a clash).
- `TestProtoEmbedded` / `TestProtoEmbeddedRecursive` — an embedded struct nests as a
  sub-message; a cyclic embed nests recursively (no violation).
- `TestViolations` — every rejected type reports the expected fix.
- `TestRenderGoWireDirect_*` — `SizeWire`/`AppendWire` emission per field shape
  (`emit_wire_direct_test.go`). Byte-exactness against `proto.Marshal` is not checked here
  (nothing compiles the output).
- `TestRenderGoWire_*` — `MarshalWire`/`ToProto` emission, the gen alias threaded (not
  hardcoded), and the output free of any `RegisterCommandCodec`/codec vestige.

protoc compilation itself (the C#/Go output) is exercised end-to-end against the
buf-in-Docker path, not by these unit tests.
