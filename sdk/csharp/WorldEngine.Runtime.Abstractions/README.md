# WorldEngine.Runtime.Abstractions

Portable contracts for game logic shared by Unity and a Cardinal backend.

## First module

Create a Unity-compatible game-logic project and install version `1.0.0`:

```bash
dotnet new classlib \
  --framework netstandard2.1 \
  --name Rampage.Gameplay \
  --output shared/Rampage.Gameplay
dotnet add shared/Rampage.Gameplay/Rampage.Gameplay.csproj \
  package WorldEngine.Runtime.Abstractions \
  --version 1.0.0
```

Define your messages in `game.proto` and generate the C# types into the shared
project with `protoc --csharp_out=shared/Rampage.Gameplay game.proto`:

```protobuf
syntax = "proto3";
package rampage.v1;
option csharp_namespace = "Rampage.Gameplay";

message GameInput { int64 value = 1; }
message GameOutput { int64 value = 1; }
message GameSnapshot { int64 value = 1; }
```

The generated types implement `Google.Protobuf.IMessage<T>`. The abstractions
package references Google.Protobuf; Unity must also reference a compatible
Google.Protobuf assembly. Enable C# 9 and nullable annotations in the shared
project for this example.

Add `shared/Rampage.Gameplay/GameModule.cs`:

```csharp
using System;
using WorldEngine.Runtime;

namespace Rampage.Gameplay
{
    public sealed class GameModule : IGameModule<GameInput, GameOutput, GameSnapshot>
    {
        private long _value;
        private readonly GameOutput _output = new GameOutput();

        public static ModuleContract Contract { get; } = new ModuleContract(
            "rampage-gameplay",
            "1.0.0");

        public GameModule(ReadOnlySpan<byte> config)
        {
            _ = config;
        }

        public void Initialize(GameSnapshot? snapshot)
        {
            _value = snapshot?.Value ?? 0;
        }

        public GameOutput Tick(in TickContext context, GameInput input)
        {
            _ = context;
            _value = unchecked(_value + input.Value);
            _output.Value = _value;
            return _output;
        }

        public GameSnapshot Snapshot() => new GameSnapshot { Value = _value };

        public void Restore(GameSnapshot snapshot) => _value = snapshot.Value;

        public void Dispose()
        {
        }
    }
}
```

Next, add the NativeAOT host described in
[`WorldEngine.Runtime.NativeAot`](https://github.com/Argus-Labs/world-engine/blob/csharp-runtime/v1.0.0/sdk/csharp/WorldEngine.Runtime.NativeAot/README.md).

## Unity install

In Unity Package Manager, choose **+ → Install package from git URL** and enter:

```text
https://github.com/Argus-Labs/world-engine.git?path=/sdk/csharp/WorldEngine.Runtime.Abstractions#csharp-runtime/v1.0.0
```

Compile the shared game-logic sources into a Unity assembly that references
`WorldEngine.Runtime.Abstractions`. Cardinal loads those same sources through
the NativeAOT host.

## Release

The NuGet and Unity package versions move together. Push a tag matching
`csharp-runtime/v<package.json version>`; the release workflow validates the
tag and publishes both NuGet packages. Unity consumers pin the same tag in the
Git URL above.

## Invariants

- No `UnityEngine`, networking, wall-clock, reflection discovery, or global RNG.
- Cardinal supplies deterministic tick time and typed protobuf input. The SDK
  handles protobuf serialization; factory configuration remains raw bytes.
- Each non-empty NativeAOT-backed system phase makes one batched call per tick;
  empty phases make none. Calls never scale with entity count.
- Native input bytes are borrowed only for the call. The SDK parses them into
  the module's declared message type.
- A module may reuse its output message. The SDK serializes it before the call
  returns into a module-owned native buffer; see the NativeAOT ownership rules.
- Throw `ModuleException` with a `RuntimeStatus` and diagnostic for a deliberate
  failure. Other exceptions become `ExecutionFailed`. A failed call does not
  guarantee rollback and must not be retried on the assumption that state is unchanged.
- Every module implements `Initialize`, `Tick`, `Snapshot`, `Restore`, and `Dispose`;
  `Tick` and `Snapshot` must return non-null messages, even when they contain no fields.
- A null initialization snapshot starts a new world. Empty protobuf bytes also
  map to null during initialization. Use `Restore` to apply an all-default snapshot.
- Mutable module state is process-local. The module must include persistent state
  in its snapshot message and restore it explicitly.
