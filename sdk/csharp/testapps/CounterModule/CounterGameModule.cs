using System;
using System.Buffers.Binary;
using WorldEngine.Runtime;

namespace WorldEngine.Runtime.CounterFixture
{
    internal sealed class CounterGameModule : IGameModule
    {
        private const int ValueSize = sizeof(long);

        private long _value;

        internal CounterGameModule(ReadOnlySpan<byte> config)
        {
            if (config.Length == ValueSize)
            {
                _value = BinaryPrimitives.ReadInt64LittleEndian(config);
            }
            else if (!config.IsEmpty)
            {
                throw new ArgumentException(
                    $"Counter config must be empty or eight bytes; received {config.Length}.",
                    nameof(config));
            }
        }

        internal static ModuleContract Contract { get; } = new ModuleContract(
            "counter-fixture",
            "1.0.0",
            RuntimeCapabilities.Initialize |
            RuntimeCapabilities.Tick |
            RuntimeCapabilities.Query |
            RuntimeCapabilities.Snapshot |
            RuntimeCapabilities.Restore,
            new byte[ModuleContract.SchemaHashLength]
            {
                0x9a, 0xaf, 0x3f, 0x8c, 0xd5, 0xc1, 0x6c, 0xa9,
                0x36, 0x91, 0x14, 0xaa, 0x09, 0x66, 0x99, 0xa2,
                0x0e, 0xbf, 0xc7, 0x4a, 0x7f, 0xf3, 0xee, 0x99,
                0x04, 0x83, 0x3d, 0x3f, 0xb9, 0xd5, 0xda, 0x30,
            });

        public RuntimeStatus Initialize(ReadOnlySpan<byte> snapshot) => Restore(snapshot);

        public RuntimeStatus Tick(
            in TickContext context,
            ReadOnlySpan<byte> input,
            Span<byte> output,
            out int outputLength)
        {
            _ = context;
            if (input.Length != ValueSize)
            {
                outputLength = 0;
                return RuntimeStatus.InvalidArgument;
            }

            outputLength = ValueSize;
            if (output.Length < outputLength)
            {
                return RuntimeStatus.BufferTooSmall;
            }

            _value += BinaryPrimitives.ReadInt64LittleEndian(input);
            BinaryPrimitives.WriteInt64LittleEndian(output, _value);
            return RuntimeStatus.Success;
        }

        public RuntimeStatus Query(
            uint kind,
            ReadOnlySpan<byte> input,
            Span<byte> output,
            out int outputLength)
        {
            if (kind != 1 || !input.IsEmpty)
            {
                outputLength = 0;
                return RuntimeStatus.Unsupported;
            }

            return WriteValue(output, out outputLength);
        }

        public RuntimeStatus Snapshot(Span<byte> output, out int outputLength) =>
            WriteValue(output, out outputLength);

        public RuntimeStatus Restore(ReadOnlySpan<byte> snapshot)
        {
            if (snapshot.IsEmpty)
            {
                return RuntimeStatus.Success;
            }

            if (snapshot.Length != ValueSize)
            {
                return RuntimeStatus.InvalidArgument;
            }

            _value = BinaryPrimitives.ReadInt64LittleEndian(snapshot);
            return RuntimeStatus.Success;
        }

        public void Dispose()
        {
        }

        private RuntimeStatus WriteValue(Span<byte> output, out int outputLength)
        {
            outputLength = ValueSize;
            if (output.Length < outputLength)
            {
                return RuntimeStatus.BufferTooSmall;
            }

            BinaryPrimitives.WriteInt64LittleEndian(output, _value);
            return RuntimeStatus.Success;
        }
    }
}
