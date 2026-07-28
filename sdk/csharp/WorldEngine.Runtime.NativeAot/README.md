# WorldEngine.Runtime.NativeAot

Build package that turns a game-specific .NET class library into the shared
library loaded by Cardinal.

## First shard build

Create the NativeAOT host and install version `1.0.0`:

```bash
dotnet new classlib \
  --framework net8.0 \
  --name Rampage.Gameplay.NativeAot \
  --output native/gameplay/Rampage.Gameplay.NativeAot
dotnet add native/gameplay/Rampage.Gameplay.NativeAot/Rampage.Gameplay.NativeAot.csproj \
  package WorldEngine.Runtime.NativeAot \
  --version 1.0.0
dotnet add native/gameplay/Rampage.Gameplay.NativeAot/Rampage.Gameplay.NativeAot.csproj \
  reference shared/Rampage.Gameplay/Rampage.Gameplay.csproj
```

The host project must name the extensionless library used by `world.toml`.
Its complete project file is:

```xml
<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
    <Nullable>enable</Nullable>
    <ImplicitUsings>disable</ImplicitUsings>
    <AssemblyName>rampage_gameplay_native</AssemblyName>
    <PublishAot>true</PublishAot>
    <NativeLib>Shared</NativeLib>
    <SelfContained>true</SelfContained>
  </PropertyGroup>
  <ItemGroup>
    <ProjectReference Include="../../../shared/Rampage.Gameplay/Rampage.Gameplay.csproj" />
    <PackageReference Include="WorldEngine.Runtime.NativeAot" Version="1.0.0" />
  </ItemGroup>
</Project>
```

Add `GameModuleFactory.cs` to the host project:

```csharp
using System;
using Rampage.Gameplay;
using WorldEngine.Runtime;

namespace WorldEngine.Runtime.NativeAot
{
    internal static partial class GameModuleFactory
    {
        internal static partial ModuleContract GetContract() => GameModule.Contract;

        internal static partial IGameModule Create(ReadOnlySpan<byte> config) =>
            new GameModule(config);
    }
}
```

The package injects all `cardinal_runtime_v1_*` exports into the published
assembly. This is required because NativeAOT exports only methods in the
published assembly, not methods in referenced packages.

Configure the shard from the world root:

```toml
[[shards]]
id = "gameplay"
path = "shards/gameplay"

[shards.native_aot]
project = "native/gameplay/Rampage.Gameplay.NativeAot/Rampage.Gameplay.NativeAot.csproj"
library = "rampage_gameplay_native"
abi_version = 1
```

`project` is world-root relative. `library` is extensionless and must equal
`<AssemblyName>`.

Build and validate the Linux/amd64 shard container image:

```bash
world build --shard gameplay
```

This command produces the shard image, not a host `.so` in the worktree. It
validates the published library and copies it into the image as
`librampage_gameplay_native.so`.

## Local library build

Publish directly when debugging the host outside World CLI:

```bash
dotnet publish \
  native/gameplay/Rampage.Gameplay.NativeAot/Rampage.Gameplay.NativeAot.csproj \
  --configuration Release \
  --runtime linux-x64 \
  --self-contained true \
  --output .world/native/gameplay
test -f .world/native/gameplay/rampage_gameplay_native.so
```

The three NativeAOT properties belong in the host project so they are available
during restore. The package validates them before build.

World CLI currently builds Linux/amd64 (`linux-x64`) shard containers. The
package itself supports standard .NET 8 NativeAOT shared-library runtime
identifiers wherever the installed .NET SDK and native toolchain support them.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Contract mismatch | Read `ContractMismatchError.Field`, `Expected`, and `Actual`; align the host requirement with `GameModule.Contract`, then rebuild. Capability values are bit masks; schema hashes are full 64-digit hex. |
| Missing `.so` | Ensure `library = "rampage_gameplay_native"` matches `<AssemblyName>`. Direct publish writes `.world/native/gameplay/rampage_gameplay_native.so`; `world build` instead copies `librampage_gameplay_native.so` into the image. |
| Missing ABI symbols | Keep the factory in namespace `WorldEngine.Runtime.NativeAot` and reference the NativeAOT package from the published host. Inspect with `nm --dynamic --defined-only .world/native/gameplay/rampage_gameplay_native.so \| grep cardinal_runtime_v1_`. |
| Wrong platform | World CLI shard images are Linux/amd64; publish with `--runtime linux-x64`. Other NativeAOT RIDs are package/toolchain capabilities, not World CLI container targets. |

The library is self-contained. Keep it loaded for the process lifetime;
NativeAOT libraries cannot be unloaded. Treat module paths as trusted
executable artifacts. If an output call returns `BufferTooSmall`, do not run
another operation on that handle before retrying with the larger buffer.
