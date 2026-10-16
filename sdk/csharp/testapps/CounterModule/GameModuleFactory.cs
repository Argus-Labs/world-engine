using System;
using System.Buffers.Binary;
using WorldEngine.Runtime;
using WorldEngine.Runtime.CounterFixture;

namespace WorldEngine.Runtime.NativeAot
{
    internal static partial class GameModuleFactory
    {
        private static readonly ModuleDefinition s_definition =
            new ModuleDefinition<FixtureInput, FixtureOutput, FixtureSnapshot>(
                CounterGameModule.Contract,
                "worldengine.cardinal.fixture.v1.FixtureInput",
                "worldengine.cardinal.fixture.v1.FixtureOutput",
                "worldengine.cardinal.fixture.v1.FixtureSnapshot",
                Create);

        internal static partial ModuleDefinition GetDefinition() => s_definition;

        private static IGameModule<FixtureInput, FixtureOutput, FixtureSnapshot> Create(ReadOnlySpan<byte> config)
        {
            if (!config.IsEmpty && config.Length != sizeof(long))
            {
                throw new ModuleException(RuntimeStatus.InvalidArgument,
                    $"Counter config must be empty or eight bytes; received {config.Length}.");
            }

            return new CounterGameModule(config.IsEmpty ? 0 : BinaryPrimitives.ReadInt64LittleEndian(config));
        }
    }
}
