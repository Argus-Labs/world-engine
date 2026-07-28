using System;
using WorldEngine.Runtime;
using WorldEngine.Runtime.CounterFixture;

namespace WorldEngine.Runtime.NativeAot
{
    internal static partial class GameModuleFactory
    {
        internal static partial ModuleContract GetContract() => CounterGameModule.Contract;

        internal static partial IGameModule Create(ReadOnlySpan<byte> config) =>
            new CounterGameModule(config);
    }
}
