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
            "1.0.0");

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
