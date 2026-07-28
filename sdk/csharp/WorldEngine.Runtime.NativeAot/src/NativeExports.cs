using System;
using System.Collections.Concurrent;
using System.Runtime.CompilerServices;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading;
using WorldEngine.Runtime;

namespace WorldEngine.Runtime.NativeAot
{
    internal static partial class GameModuleFactory
    {
        internal static partial ModuleContract GetContract();

        internal static partial IGameModule Create(ReadOnlySpan<byte> config);
    }

    internal sealed class ModuleEntry
    {
        internal ModuleEntry(IGameModule module)
        {
            Module = module;
        }

        internal object Gate { get; } = new object();

        internal IGameModule Module { get; }

        internal string? LastError { get; set; }
    }

    [StructLayout(LayoutKind.Sequential)]
    internal unsafe struct NativeContract
    {
        internal uint AbiVersion;
        internal uint StructSize;
        internal ulong Capabilities;
        internal fixed byte SchemaHash[ModuleContract.SchemaHashLength];
        internal fixed byte Name[64];
        internal fixed byte Version[32];
        internal fixed ulong Reserved[4];
    }

    internal static unsafe class NativeExports
    {
        private const uint AbiVersion = 1;
        private const int NameCapacity = 64;
        private const int VersionCapacity = 32;

        private static readonly ConcurrentDictionary<ulong, ModuleEntry> s_modules = new();
        private static long s_nextHandle;
        [ThreadStatic]
        private static string? t_globalError;

        [UnmanagedCallersOnly(
            EntryPoint = "cardinal_runtime_v1_get_contract",
            CallConvs = new[] { typeof(CallConvCdecl) })]
        internal static int GetContract(NativeContract* output)
        {
            if (output == null)
            {
                return (int)RuntimeStatus.InvalidArgument;
            }

            try
            {
                ModuleContract contract = GameModuleFactory.GetContract();
                if (contract.SchemaHash.Length != ModuleContract.SchemaHashLength)
                {
                    throw new InvalidOperationException(
                        $"Schema hash must contain {ModuleContract.SchemaHashLength} bytes.");
                }

                *output = default;
                output->AbiVersion = AbiVersion;
                output->StructSize = (uint)sizeof(NativeContract);
                output->Capabilities = (ulong)contract.Capabilities;

                contract.SchemaHash.Span.CopyTo(
                    new Span<byte>(output->SchemaHash, ModuleContract.SchemaHashLength));
                WriteNullTerminatedUtf8(
                    contract.Name,
                    new Span<byte>(output->Name, NameCapacity),
                    nameof(contract.Name));
                WriteNullTerminatedUtf8(
                    contract.Version,
                    new Span<byte>(output->Version, VersionCapacity),
                    nameof(contract.Version));

                SetGlobalError(null);
                return (int)RuntimeStatus.Success;
            }
            catch (Exception exception)
            {
                SetGlobalError(FormatException(exception));
                return (int)RuntimeStatus.ExecutionFailed;
            }
        }

        [UnmanagedCallersOnly(
            EntryPoint = "cardinal_runtime_v1_create",
            CallConvs = new[] { typeof(CallConvCdecl) })]
        internal static int Create(
            byte* config,
            nuint configLength,
            ulong* outputHandle)
        {
            if (outputHandle == null || !TryCreateReadOnlySpan(config, configLength, out var configSpan))
            {
                return (int)RuntimeStatus.InvalidArgument;
            }

            *outputHandle = 0;

            try
            {
                IGameModule module = GameModuleFactory.Create(configSpan)
                    ?? throw new InvalidOperationException("Game module factory returned null.");
                ulong handle = NextHandle();
                if (!s_modules.TryAdd(handle, new ModuleEntry(module)))
                {
                    module.Dispose();
                    throw new InvalidOperationException("Failed to allocate a unique module handle.");
                }

                *outputHandle = handle;
                SetGlobalError(null);
                return (int)RuntimeStatus.Success;
            }
            catch (Exception exception)
            {
                SetGlobalError(FormatException(exception));
                return (int)RuntimeStatus.ExecutionFailed;
            }
        }

        [UnmanagedCallersOnly(
            EntryPoint = "cardinal_runtime_v1_initialize",
            CallConvs = new[] { typeof(CallConvCdecl) })]
        internal static int Initialize(
            ulong handle,
            byte* snapshot,
            nuint snapshotLength)
        {
            if (!TryCreateReadOnlySpan(snapshot, snapshotLength, out var snapshotSpan))
            {
                return (int)RuntimeStatus.InvalidArgument;
            }

            if (!s_modules.TryGetValue(handle, out ModuleEntry? entry))
            {
                return (int)RuntimeStatus.InvalidHandle;
            }

            lock (entry.Gate)
            {
                try
                {
                    RuntimeStatus status = entry.Module.Initialize(snapshotSpan);
                    SetModuleStatus(entry, status);
                    return (int)status;
                }
                catch (Exception exception)
                {
                    entry.LastError = FormatException(exception);
                    return (int)RuntimeStatus.ExecutionFailed;
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
            nuint inputLength,
            byte* output,
            nuint outputCapacity,
            nuint* outputLength)
        {
            if (!TryCreateReadOnlySpan(input, inputLength, out var inputSpan) ||
                !TryCreateSpan(output, outputCapacity, out var outputSpan) ||
                outputLength == null)
            {
                return (int)RuntimeStatus.InvalidArgument;
            }

            *outputLength = 0;
            TickContext context = new TickContext(tick, fixedDeltaNanoseconds);
            if (!s_modules.TryGetValue(handle, out ModuleEntry? entry))
            {
                return (int)RuntimeStatus.InvalidHandle;
            }

            lock (entry.Gate)
            {
                try
                {
                    RuntimeStatus status = entry.Module.Tick(
                        in context,
                        inputSpan,
                        outputSpan,
                        out int length);
                    return CompleteOutputStatus(
                        entry,
                        status,
                        length,
                        outputSpan.Length,
                        outputLength);
                }
                catch (Exception exception)
                {
                    entry.LastError = FormatException(exception);
                    return (int)RuntimeStatus.ExecutionFailed;
                }
            }
        }

        [UnmanagedCallersOnly(
            EntryPoint = "cardinal_runtime_v1_query",
            CallConvs = new[] { typeof(CallConvCdecl) })]
        internal static int Query(
            ulong handle,
            uint kind,
            byte* input,
            nuint inputLength,
            byte* output,
            nuint outputCapacity,
            nuint* outputLength)
        {
            if (!TryCreateReadOnlySpan(input, inputLength, out var inputSpan) ||
                !TryCreateSpan(output, outputCapacity, out var outputSpan) ||
                outputLength == null)
            {
                return (int)RuntimeStatus.InvalidArgument;
            }

            *outputLength = 0;
            if (!s_modules.TryGetValue(handle, out ModuleEntry? entry))
            {
                return (int)RuntimeStatus.InvalidHandle;
            }

            lock (entry.Gate)
            {
                try
                {
                    RuntimeStatus status = entry.Module.Query(
                        kind,
                        inputSpan,
                        outputSpan,
                        out int length);
                    return CompleteOutputStatus(
                        entry,
                        status,
                        length,
                        outputSpan.Length,
                        outputLength);
                }
                catch (Exception exception)
                {
                    entry.LastError = FormatException(exception);
                    return (int)RuntimeStatus.ExecutionFailed;
                }
            }
        }

        [UnmanagedCallersOnly(
            EntryPoint = "cardinal_runtime_v1_snapshot",
            CallConvs = new[] { typeof(CallConvCdecl) })]
        internal static int Snapshot(
            ulong handle,
            byte* output,
            nuint outputCapacity,
            nuint* outputLength)
        {
            if (!TryCreateSpan(output, outputCapacity, out var outputSpan) ||
                outputLength == null)
            {
                return (int)RuntimeStatus.InvalidArgument;
            }

            *outputLength = 0;
            if (!s_modules.TryGetValue(handle, out ModuleEntry? entry))
            {
                return (int)RuntimeStatus.InvalidHandle;
            }

            lock (entry.Gate)
            {
                try
                {
                    RuntimeStatus status = entry.Module.Snapshot(outputSpan, out int length);
                    return CompleteOutputStatus(
                        entry,
                        status,
                        length,
                        outputSpan.Length,
                        outputLength);
                }
                catch (Exception exception)
                {
                    entry.LastError = FormatException(exception);
                    return (int)RuntimeStatus.ExecutionFailed;
                }
            }
        }

        [UnmanagedCallersOnly(
            EntryPoint = "cardinal_runtime_v1_restore",
            CallConvs = new[] { typeof(CallConvCdecl) })]
        internal static int Restore(
            ulong handle,
            byte* snapshot,
            nuint snapshotLength)
        {
            if (!TryCreateReadOnlySpan(snapshot, snapshotLength, out var snapshotSpan))
            {
                return (int)RuntimeStatus.InvalidArgument;
            }

            if (!s_modules.TryGetValue(handle, out ModuleEntry? entry))
            {
                return (int)RuntimeStatus.InvalidHandle;
            }

            lock (entry.Gate)
            {
                try
                {
                    RuntimeStatus status = entry.Module.Restore(snapshotSpan);
                    SetModuleStatus(entry, status);
                    return (int)status;
                }
                catch (Exception exception)
                {
                    entry.LastError = FormatException(exception);
                    return (int)RuntimeStatus.ExecutionFailed;
                }
            }
        }

        [UnmanagedCallersOnly(
            EntryPoint = "cardinal_runtime_v1_last_error",
            CallConvs = new[] { typeof(CallConvCdecl) })]
        internal static int LastError(
            ulong handle,
            byte* output,
            nuint outputCapacity,
            nuint* outputLength)
        {
            if (!TryCreateSpan(output, outputCapacity, out var outputSpan) ||
                outputLength == null)
            {
                return (int)RuntimeStatus.InvalidArgument;
            }

            try
            {
                string error = GetError(handle) ?? string.Empty;
                int requiredLength = Encoding.UTF8.GetByteCount(error);
                *outputLength = (nuint)requiredLength;
                if (outputSpan.Length < requiredLength)
                {
                    return (int)RuntimeStatus.BufferTooSmall;
                }

                Encoding.UTF8.GetBytes(error.AsSpan(), outputSpan);
                return (int)RuntimeStatus.Success;
            }
            catch (Exception exception)
            {
                SetGlobalError(FormatException(exception));
                return (int)RuntimeStatus.ExecutionFailed;
            }
        }

        [UnmanagedCallersOnly(
            EntryPoint = "cardinal_runtime_v1_destroy",
            CallConvs = new[] { typeof(CallConvCdecl) })]
        internal static int Destroy(ulong handle)
        {
            if (handle == 0 || !s_modules.TryRemove(handle, out ModuleEntry? entry))
            {
                return (int)RuntimeStatus.InvalidHandle;
            }

            try
            {
                lock (entry.Gate)
                {
                    entry.Module.Dispose();
                    entry.LastError = null;
                }

                return (int)RuntimeStatus.Success;
            }
            catch (Exception exception)
            {
                SetGlobalError(FormatException(exception));
                return (int)RuntimeStatus.ExecutionFailed;
            }
        }

        private static int CompleteOutputStatus(
            ModuleEntry entry,
            RuntimeStatus status,
            int outputLength,
            int outputCapacity,
            nuint* nativeOutputLength)
        {
            if (outputLength < 0 ||
                (status == RuntimeStatus.Success && outputLength > outputCapacity))
            {
                throw new InvalidOperationException(
                    $"Module returned invalid output length {outputLength} for capacity {outputCapacity}.");
            }

            *nativeOutputLength = (nuint)outputLength;
            SetModuleStatus(entry, status);
            return (int)status;
        }

        private static void SetModuleStatus(ModuleEntry entry, RuntimeStatus status)
        {
            entry.LastError = status == RuntimeStatus.Success ||
                              status == RuntimeStatus.BufferTooSmall
                ? null
                : $"Module returned {status}.";
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

        private static bool TryCreateReadOnlySpan(
            byte* data,
            nuint length,
            out ReadOnlySpan<byte> span)
        {
            if (length > int.MaxValue || (data == null && length != 0))
            {
                span = default;
                return false;
            }

            span = new ReadOnlySpan<byte>(data, checked((int)length));
            return true;
        }

        private static bool TryCreateSpan(
            byte* data,
            nuint capacity,
            out Span<byte> span)
        {
            if (capacity > int.MaxValue || (data == null && capacity != 0))
            {
                span = default;
                return false;
            }

            span = new Span<byte>(data, checked((int)capacity));
            return true;
        }

        private static void WriteNullTerminatedUtf8(
            string value,
            Span<byte> destination,
            string fieldName)
        {
            int byteCount = Encoding.UTF8.GetByteCount(value);
            if (byteCount >= destination.Length)
            {
                throw new InvalidOperationException(
                    $"{fieldName} needs {byteCount + 1} bytes; maximum is {destination.Length}.");
            }

            destination.Clear();
            Encoding.UTF8.GetBytes(value.AsSpan(), destination);
        }

        private static string FormatException(Exception exception) =>
            $"{exception.GetType().Name}: {exception.Message}";

    }
}
