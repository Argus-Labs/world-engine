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

Start with a stateless module in
`shared/Rampage.Gameplay/GameModule.cs`. Replace the zero schema hash with the
SHA-256 of the real binary input/output schema before production:

```csharp
using System;
using WorldEngine.Runtime;

namespace Rampage.Gameplay
{
    public sealed class GameModule : IGameModule
    {
        public static ModuleContract Contract { get; } = new ModuleContract(
            "rampage-gameplay",
            "1.0.0",
            RuntimeCapabilities.Initialize |
            RuntimeCapabilities.Tick |
            RuntimeCapabilities.Stateless,
            new byte[ModuleContract.SchemaHashLength]);

        public GameModule(ReadOnlySpan<byte> config)
        {
            _ = config;
        }

        public RuntimeStatus Initialize(ReadOnlySpan<byte> snapshot) =>
            snapshot.IsEmpty ? RuntimeStatus.Success : RuntimeStatus.Unsupported;

        public RuntimeStatus Tick(
            in TickContext context,
            ReadOnlySpan<byte> input,
            Span<byte> output,
            out int outputLength)
        {
            _ = context;
            _ = input;
            _ = output;
            outputLength = 0;
            return RuntimeStatus.Success;
        }

        public RuntimeStatus Query(
            uint kind,
            ReadOnlySpan<byte> input,
            Span<byte> output,
            out int outputLength)
        {
            _ = kind;
            _ = input;
            _ = output;
            outputLength = 0;
            return RuntimeStatus.Unsupported;
        }

        public RuntimeStatus Snapshot(Span<byte> output, out int outputLength)
        {
            _ = output;
            outputLength = 0;
            return RuntimeStatus.Unsupported;
        }

        public RuntimeStatus Restore(ReadOnlySpan<byte> snapshot) =>
            snapshot.IsEmpty ? RuntimeStatus.Success : RuntimeStatus.Unsupported;

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

The NuGet and Unity package versions move together. C# runtime packages are
released from `main`; push `csharp-runtime/v<package.json version>` at the
matching main commit. The release workflow validates the tag and publishes both
NuGet packages. Unity consumers pin the same tag in the Git URL above.

## Invariants

- No `UnityEngine`, networking, wall-clock, reflection discovery, or global RNG.
- Cardinal supplies deterministic tick time and opaque binary input.
- Each non-empty NativeAOT-backed system phase makes one batched call per tick;
  empty phases make none. Calls never scale with entity count.
- Input memory is borrowed only for the call.
- Output memory belongs to the caller and should be reused.
- `BufferTooSmall` leaves module state and output unchanged; the host may retry.
- Stateful modules advertise `Snapshot` and make all authoritative state restorable.
- A `Stateless` module keeps authoritative state in Cardinal ECS.
