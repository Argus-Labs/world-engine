using System;

namespace WorldEngine.Runtime
{
    /// <summary>
    /// Deterministic game logic hosted in-process by Cardinal.
    /// </summary>
    /// <remarks>
    /// Inputs are borrowed for the duration of a call. Outputs are written into
    /// caller-owned memory. Implementations must set <c>outputLength</c> to the
    /// required size when returning <see cref="RuntimeStatus.BufferTooSmall"/>.
    /// A buffer-too-small call must not mutate module state or output because
    /// the host can retry it with a larger buffer.
    /// Calls for one module instance are serialized by the host.
    /// </remarks>
    public interface IGameModule : IDisposable
    {
        RuntimeStatus Initialize(ReadOnlySpan<byte> snapshot);

        RuntimeStatus Tick(
            in TickContext context,
            ReadOnlySpan<byte> input,
            Span<byte> output,
            out int outputLength);

        RuntimeStatus Query(
            uint kind,
            ReadOnlySpan<byte> input,
            Span<byte> output,
            out int outputLength);

        RuntimeStatus Snapshot(Span<byte> output, out int outputLength);

        RuntimeStatus Restore(ReadOnlySpan<byte> snapshot);
    }
}
