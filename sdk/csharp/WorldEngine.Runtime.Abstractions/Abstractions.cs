using System;
using Google.Protobuf;

namespace WorldEngine.Runtime
{
    /// <summary>
    /// Defines deterministic game logic. Cardinal runs this logic in the host process.
    /// </summary>
    /// <remarks>
    /// Module logic consumes and produces typed protobuf messages. The SDK handles serialization.
    /// The host serializes calls for one module instance. Throw <see cref="ModuleException"/> to
    /// report a deliberate runtime status.
    /// </remarks>
    public interface IGameModule<TInput, TOutput, TSnapshot> : IDisposable
        where TInput : IMessage<TInput>, new()
        where TOutput : IMessage<TOutput>, new()
        where TSnapshot : IMessage<TSnapshot>, new()
    {
        /// <summary>
        /// Initializes the module. A null snapshot starts a new world.
        /// </summary>
        void Initialize(TSnapshot? snapshot);

        TOutput Tick(in TickContext context, TInput input);

        TSnapshot Snapshot();

        void Restore(TSnapshot snapshot);
    }

    /// <summary>
    /// Identifies a module. Cardinal validates this identity before it creates a module instance.
    /// </summary>
    public readonly struct ModuleContract
    {
        public ModuleContract(
            string name,
            string version)
        {
            if (string.IsNullOrWhiteSpace(name))
            {
                throw new ArgumentException("Module name is required.", nameof(name));
            }

            if (string.IsNullOrWhiteSpace(version))
            {
                throw new ArgumentException("Module version is required.", nameof(version));
            }

            Name = name;
            Version = version;
        }

        public string Name { get; }

        public string Version { get; }
    }

    /// <summary>
    /// Contains deterministic time that Cardinal supplies. A module must not read the system clock.
    /// </summary>
    public readonly struct TickContext
    {
        public TickContext(ulong tick, ulong fixedDeltaNanoseconds)
        {
            Tick = tick;
            FixedDeltaNanoseconds = fixedDeltaNanoseconds;
        }

        public ulong Tick { get; }

        public ulong FixedDeltaNanoseconds { get; }
    }

    /// <summary>
    /// Defines stable status values for the Cardinal native runtime ABI.
    /// </summary>
    public enum RuntimeStatus
    {
        Success = 0,
        InvalidArgument = 2,
        InvalidHandle = 3,
        InvalidState = 4,
        Unsupported = 5,
        ExecutionFailed = 6,
        AbiMismatch = 7,
    }

    /// <summary>
    /// Reports a deliberate module failure with a native runtime status and diagnostic message.
    /// </summary>
    public sealed class ModuleException : Exception
    {
        public ModuleException(RuntimeStatus status, string message)
            : base(message)
        {
            Status = status;
        }

        public RuntimeStatus Status { get; }
    }
}
