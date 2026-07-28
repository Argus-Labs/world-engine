namespace WorldEngine.Runtime
{
    /// <summary>
    /// Deterministic time supplied by Cardinal. Modules must not read wall-clock time.
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
}
