using System;
using System.Collections.Concurrent;
using System.Diagnostics;
using System.Runtime.CompilerServices;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading;
using Google.Protobuf;
using WorldEngine.Runtime;

namespace WorldEngine.Runtime.NativeAot
{
    internal static partial class GameModuleFactory
    {
        internal static partial ModuleDefinition GetDefinition();
    }

    internal abstract class ModuleDefinition
    {
        protected ModuleDefinition(ModuleContract contract, string inputType, string outputType, string snapshotType)
        {
            Contract = contract;
            InputType = inputType;
            OutputType = outputType;
            SnapshotType = snapshotType;
        }

        internal ModuleContract Contract { get; }
        internal string InputType { get; }
        internal string OutputType { get; }
        internal string SnapshotType { get; }
        internal abstract ModuleEntry Create(ReadOnlySpan<byte> config);
    }

    internal sealed class ModuleDefinition<TInput, TOutput, TSnapshot> : ModuleDefinition
        where TInput : class, IMessage<TInput>, new()
        where TOutput : class, IMessage<TOutput>, new()
        where TSnapshot : class, IMessage<TSnapshot>, new()
    {
        internal delegate IGameModule<TInput, TOutput, TSnapshot> Factory(ReadOnlySpan<byte> config);
        private readonly Factory _factory;

        internal ModuleDefinition(ModuleContract contract, string inputType, string outputType,
            string snapshotType, Factory factory)
            : base(contract, inputType, outputType, snapshotType)
        {
            _factory = factory;
        }

        internal override ModuleEntry Create(ReadOnlySpan<byte> config) =>
            new TypedModuleEntry(_factory(config)
                ?? throw new InvalidOperationException("Game module factory returned null."));

        private sealed class TypedModuleEntry : ModuleEntry
        {
            private static readonly MessageParser<TInput> s_inputParser = new(() => new TInput());
            private static readonly MessageParser<TSnapshot> s_snapshotParser = new(() => new TSnapshot());
            private readonly IGameModule<TInput, TOutput, TSnapshot> _module;

            internal TypedModuleEntry(IGameModule<TInput, TOutput, TSnapshot> module) => _module = module;

            internal override void Initialize(ReadOnlySpan<byte> snapshot) =>
                _module.Initialize(snapshot.IsEmpty ? null : s_snapshotParser.ParseFrom(snapshot));

            internal override IMessage Tick(in TickContext context, ReadOnlySpan<byte> input) =>
                _module.Tick(in context, s_inputParser.ParseFrom(input));

            internal override IMessage Snapshot() => _module.Snapshot();

            internal override void Restore(ReadOnlySpan<byte> snapshot) =>
                _module.Restore(s_snapshotParser.ParseFrom(snapshot));

            protected override void DisposeModule() => _module.Dispose();
        }
    }

    internal abstract class ModuleEntry : IDisposable
    {
        private byte[] _output = Array.Empty<byte>();
        private GCHandle _outputPin;

        internal object Gate { get; } = new object();
        internal string? LastError { get; set; }

        internal abstract void Initialize(ReadOnlySpan<byte> snapshot);
        internal abstract IMessage Tick(in TickContext context, ReadOnlySpan<byte> input);
        internal abstract IMessage Snapshot();
        internal abstract void Restore(ReadOnlySpan<byte> snapshot);
        protected abstract void DisposeModule();

        internal unsafe void Serialize(IMessage message, byte** output, ulong* outputLength)
        {
            int length = message.CalculateSize();
            if (length > _output.Length)
            {
                byte[] replacement = new byte[length];
                GCHandle replacementPin = GCHandle.Alloc(replacement, GCHandleType.Pinned);
                if (_outputPin.IsAllocated)
                {
                    _outputPin.Free();
                }

                _output = replacement;
                _outputPin = replacementPin;
            }

            message.WriteTo(_output.AsSpan(0, length));
            *output = _outputPin.IsAllocated ? (byte*)_outputPin.AddrOfPinnedObject() : null;
            *outputLength = (ulong)length;
        }

        public void Dispose()
        {
            try
            {
                DisposeModule();
            }
            finally
            {
                if (_outputPin.IsAllocated)
                {
                    _outputPin.Free();
                }
            }
        }
    }

    [StructLayout(LayoutKind.Sequential)]
    internal unsafe struct NativeContract
    {
        internal uint AbiVersion;
        internal fixed byte Name[64];
        internal fixed byte Version[32];
        internal fixed byte InputType[128];
        internal fixed byte OutputType[128];
        internal fixed byte SnapshotType[128];
    }

    internal static unsafe class NativeExports
    {
        private const uint AbiVersion = 1;
        private const int NameCapacity = 64;
        private const int VersionCapacity = 32;
        private const int TypeCapacity = 128;
        private const int LastErrorCapacity = 1024;

        private static readonly ConcurrentDictionary<ulong, ModuleEntry> s_modules = new();
        private static long s_nextHandle;
        [ThreadStatic]
        private static string? t_globalError;

        [UnmanagedCallersOnly(
            EntryPoint = "cardinal_runtime_v1_get_contract",
            CallConvs = new[] { typeof(CallConvCdecl) })]
        internal static int GetContract(NativeContract* output)
        {
            Debug.Assert(output != null);

            try
            {
                ModuleDefinition definition = GameModuleFactory.GetDefinition();
                ModuleContract contract = definition.Contract;
                *output = default;
                output->AbiVersion = AbiVersion;

                WriteNullTerminatedUtf8(
                    contract.Name,
                    new Span<byte>(output->Name, NameCapacity),
                    nameof(contract.Name));
                WriteNullTerminatedUtf8(
                    contract.Version,
                    new Span<byte>(output->Version, VersionCapacity),
                    nameof(contract.Version));
                WriteNullTerminatedUtf8(definition.InputType,
                    new Span<byte>(output->InputType, TypeCapacity), nameof(definition.InputType));
                WriteNullTerminatedUtf8(definition.OutputType,
                    new Span<byte>(output->OutputType, TypeCapacity), nameof(definition.OutputType));
                WriteNullTerminatedUtf8(definition.SnapshotType,
                    new Span<byte>(output->SnapshotType, TypeCapacity), nameof(definition.SnapshotType));

                SetGlobalError(null);
                return (int)RuntimeStatus.Success;
            }
            catch (Exception exception)
            {
                SetGlobalError(FormatException(exception));
                return ExceptionStatus(exception);
            }
        }

        [UnmanagedCallersOnly(
            EntryPoint = "cardinal_runtime_v1_create",
            CallConvs = new[] { typeof(CallConvCdecl) })]
        internal static int Create(
            byte* config,
            ulong configLength,
            ulong* outputHandle)
        {
            Debug.Assert(outputHandle != null);

            ReadOnlySpan<byte> configSpan = CreateReadOnlySpan(config, configLength);
            *outputHandle = 0;

            try
            {
                ModuleEntry entry = GameModuleFactory.GetDefinition().Create(configSpan);
                ulong handle = NextHandle();
                bool added = s_modules.TryAdd(handle, entry);
                Debug.Assert(added);

                *outputHandle = handle;
                SetGlobalError(null);
                return (int)RuntimeStatus.Success;
            }
            catch (Exception exception)
            {
                SetGlobalError(FormatException(exception));
                return ExceptionStatus(exception);
            }
        }

        [UnmanagedCallersOnly(
            EntryPoint = "cardinal_runtime_v1_initialize",
            CallConvs = new[] { typeof(CallConvCdecl) })]
        internal static int Initialize(
            ulong handle,
            byte* snapshot,
            ulong snapshotLength)
        {
            Debug.Assert(handle != 0);

            ReadOnlySpan<byte> snapshotSpan = CreateReadOnlySpan(snapshot, snapshotLength);
            bool found = s_modules.TryGetValue(handle, out ModuleEntry? entry);
            Debug.Assert(found);
            Debug.Assert(entry != null);

            lock (entry.Gate)
            {
                try
                {
                    entry.Initialize(snapshotSpan);
                    entry.LastError = null;
                    return (int)RuntimeStatus.Success;
                }
                catch (Exception exception)
                {
                    entry.LastError = FormatException(exception);
                    return ExceptionStatus(exception);
                }
            }
        }

        [UnmanagedCallersOnly(
            EntryPoint = "cardinal_runtime_v1_tick",
            CallConvs = new[] { typeof(CallConvCdecl) })]
        internal static int Tick(
            ulong handle,
            ulong tick,
            ulong fixedDeltaNanoseconds,
            byte* input,
            ulong inputLength,
            byte** output,
            ulong* outputLength)
        {
            Debug.Assert(handle != 0);
            Debug.Assert(output != null);
            Debug.Assert(outputLength != null);

            ReadOnlySpan<byte> inputSpan = CreateReadOnlySpan(input, inputLength);
            *output = null;
            *outputLength = 0;
            TickContext context = new TickContext(tick, fixedDeltaNanoseconds);
            bool found = s_modules.TryGetValue(handle, out ModuleEntry? entry);
            Debug.Assert(found);
            Debug.Assert(entry != null);

            lock (entry.Gate)
            {
                try
                {
                    entry.Serialize(entry.Tick(in context, inputSpan), output, outputLength);
                    entry.LastError = null;
                    return (int)RuntimeStatus.Success;
                }
                catch (Exception exception)
                {
                    entry.LastError = FormatException(exception);
                    return ExceptionStatus(exception);
                }
            }
        }

        [UnmanagedCallersOnly(
            EntryPoint = "cardinal_runtime_v1_snapshot",
            CallConvs = new[] { typeof(CallConvCdecl) })]
        internal static int Snapshot(
            ulong handle,
            byte** output,
            ulong* outputLength)
        {
            Debug.Assert(handle != 0);
            Debug.Assert(output != null);
            Debug.Assert(outputLength != null);

            *output = null;
            *outputLength = 0;
            bool found = s_modules.TryGetValue(handle, out ModuleEntry? entry);
            Debug.Assert(found);
            Debug.Assert(entry != null);

            lock (entry.Gate)
            {
                try
                {
                    entry.Serialize(entry.Snapshot(), output, outputLength);
                    entry.LastError = null;
                    return (int)RuntimeStatus.Success;
                }
                catch (Exception exception)
                {
                    entry.LastError = FormatException(exception);
                    return ExceptionStatus(exception);
                }
            }
        }

        [UnmanagedCallersOnly(
            EntryPoint = "cardinal_runtime_v1_restore",
            CallConvs = new[] { typeof(CallConvCdecl) })]
        internal static int Restore(
            ulong handle,
            byte* snapshot,
            ulong snapshotLength)
        {
            Debug.Assert(handle != 0);

            ReadOnlySpan<byte> snapshotSpan = CreateReadOnlySpan(snapshot, snapshotLength);
            bool found = s_modules.TryGetValue(handle, out ModuleEntry? entry);
            Debug.Assert(found);
            Debug.Assert(entry != null);

            lock (entry.Gate)
            {
                try
                {
                    entry.Restore(snapshotSpan);
                    entry.LastError = null;
                    return (int)RuntimeStatus.Success;
                }
                catch (Exception exception)
                {
                    entry.LastError = FormatException(exception);
                    return ExceptionStatus(exception);
                }
            }
        }

        [UnmanagedCallersOnly(
            EntryPoint = "cardinal_runtime_v1_last_error",
            CallConvs = new[] { typeof(CallConvCdecl) })]
        internal static int LastError(
            ulong handle,
            byte* output,
            ulong outputCapacity,
            ulong* outputLength)
        {
            Debug.Assert(output != null);
            Debug.Assert(outputCapacity == LastErrorCapacity);
            Debug.Assert(outputLength != null);

            Span<byte> outputSpan = CreateSpan(output, outputCapacity);
            try
            {
                string error = GetError(handle) ?? string.Empty;
                Encoding.UTF8.GetEncoder().Convert(
                    error.AsSpan(),
                    outputSpan,
                    true,
                    out _,
                    out int bytesWritten,
                    out _);
                *outputLength = (ulong)bytesWritten;
                return (int)RuntimeStatus.Success;
            }
            catch (Exception exception)
            {
                SetGlobalError(FormatException(exception));
                return ExceptionStatus(exception);
            }
        }

        [UnmanagedCallersOnly(
            EntryPoint = "cardinal_runtime_v1_destroy",
            CallConvs = new[] { typeof(CallConvCdecl) })]
        internal static int Destroy(ulong handle)
        {
            Debug.Assert(handle != 0);

            bool removed = s_modules.TryRemove(handle, out ModuleEntry? entry);
            Debug.Assert(removed);
            Debug.Assert(entry != null);

            try
            {
                lock (entry.Gate)
                {
                    entry.Dispose();
                    entry.LastError = null;
                }

                return (int)RuntimeStatus.Success;
            }
            catch (Exception exception)
            {
                SetGlobalError(FormatException(exception));
                return ExceptionStatus(exception);
            }
        }

        private static string? GetError(ulong handle)
        {
            if (handle != 0 && s_modules.TryGetValue(handle, out ModuleEntry? entry))
            {
                lock (entry.Gate)
                {
                    return entry.LastError;
                }
            }

            return t_globalError;
        }

        private static void SetGlobalError(string? error) => t_globalError = error;

        private static ulong NextHandle()
        {
            long handle = Interlocked.Increment(ref s_nextHandle);
            if (handle <= 0)
            {
                throw new InvalidOperationException("Module handle space was exhausted.");
            }

            return (ulong)handle;
        }

        private static ReadOnlySpan<byte> CreateReadOnlySpan(
            byte* data,
            ulong length)
        {
            Debug.Assert(data != null || length == 0);
            Debug.Assert(length <= int.MaxValue);

            return new ReadOnlySpan<byte>(data, checked((int)length));
        }

        private static Span<byte> CreateSpan(
            byte* data,
            ulong capacity)
        {
            Debug.Assert(data != null || capacity == 0);
            Debug.Assert(capacity <= int.MaxValue);

            return new Span<byte>(data, checked((int)capacity));
        }

        private static void WriteNullTerminatedUtf8(
            string value,
            Span<byte> destination,
            string fieldName)
        {
            if (string.IsNullOrWhiteSpace(value) || value.Contains('\0'))
            {
                throw new InvalidOperationException($"{fieldName} must be non-empty and contain no NUL characters.");
            }

            int byteCount = Encoding.UTF8.GetByteCount(value);
            if (byteCount >= destination.Length)
            {
                throw new InvalidOperationException(
                    $"{fieldName} needs {byteCount + 1} bytes; maximum is {destination.Length}.");
            }

            destination.Clear();
            Encoding.UTF8.GetBytes(value.AsSpan(), destination);
        }

        private static string FormatException(Exception exception) => exception is ModuleException
            ? exception.Message
            : $"{exception.GetType().Name}: {exception.Message}";

        private static int ExceptionStatus(Exception exception) => (int)(exception switch
        {
            ModuleException moduleException => moduleException.Status,
            InvalidProtocolBufferException => RuntimeStatus.InvalidArgument,
            _ => RuntimeStatus.ExecutionFailed,
        });

    }
}
