using System;

namespace WorldEngine.Runtime
{
    /// <summary>
    /// Identity validated before a module instance is created.
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
}
