namespace WorldEngine.Runtime
{
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
}
