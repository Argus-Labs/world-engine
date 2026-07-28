using System;

namespace WorldEngine.Runtime
{
    /// <summary>
    /// Optional operations implemented by a game module.
    /// </summary>
    [Flags]
    public enum RuntimeCapabilities : ulong
    {
        None = 0,
        Initialize = 1UL << 0,
        Tick = 1UL << 1,
        Query = 1UL << 2,
        Snapshot = 1UL << 3,
        Restore = 1UL << 4,
        Stateless = 1UL << 5,
    }
}
