using System;

namespace WorldEngine.Runtime
{
    /// <summary>
    /// Defines deterministic game logic. Cardinal runs this logic in the Cardinal process.
    /// </summary>
    /// <remarks>
    /// The caller owns each output buffer. The module borrows each input and output buffer for one
    /// call. If an output buffer is too small, the module must set <c>outputLength</c> to the required
    /// size. It must then return <see cref="RuntimeStatus.BufferTooSmall"/>. A call that returns this
    /// status must not change the module state or the output buffer. The host serializes calls for
    /// one module instance.
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
