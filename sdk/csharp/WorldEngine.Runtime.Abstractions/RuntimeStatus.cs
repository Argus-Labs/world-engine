namespace WorldEngine.Runtime
{
    /// <summary>
    /// Stable status values shared with the Cardinal native runtime ABI.
    /// </summary>
    public enum RuntimeStatus
    {
        Success = 0,
        BufferTooSmall = 1,
        InvalidArgument = 2,
        InvalidHandle = 3,
        InvalidState = 4,
        Unsupported = 5,
        ExecutionFailed = 6,
        AbiMismatch = 7,
    }
}
