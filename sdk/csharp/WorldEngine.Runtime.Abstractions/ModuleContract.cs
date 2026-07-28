using System;

namespace WorldEngine.Runtime
{
    /// <summary>
    /// Version and schema metadata validated before a module instance is created.
    /// </summary>
    public readonly struct ModuleContract
    {
        public const int SchemaHashLength = 32;

        public ModuleContract(
            string name,
            string version,
            RuntimeCapabilities capabilities,
            byte[] schemaHash)
        {
            if (string.IsNullOrWhiteSpace(name))
            {
                throw new ArgumentException("Module name is required.", nameof(name));
            }

            if (string.IsNullOrWhiteSpace(version))
            {
                throw new ArgumentException("Module version is required.", nameof(version));
            }

            if (schemaHash == null)
            {
                throw new ArgumentNullException(nameof(schemaHash));
            }

            if (schemaHash.Length != SchemaHashLength)
            {
                throw new ArgumentException(
                    $"Schema hash must contain exactly {SchemaHashLength} bytes.",
                    nameof(schemaHash));
            }

            Name = name;
            Version = version;
            Capabilities = capabilities;
            SchemaHash = new ReadOnlyMemory<byte>((byte[])schemaHash.Clone());
        }

        public string Name { get; }

        public string Version { get; }

        public RuntimeCapabilities Capabilities { get; }

        /// <summary>
        /// SHA-256 of the module's binary input/output schema.
        /// </summary>
        public ReadOnlyMemory<byte> SchemaHash { get; }
    }
}
