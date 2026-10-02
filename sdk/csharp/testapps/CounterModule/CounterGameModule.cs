using WorldEngine.Runtime;

namespace WorldEngine.Runtime.CounterFixture
{
    internal sealed class CounterGameModule : IGameModule<FixtureInput, FixtureOutput, FixtureSnapshot>
    {
        private long _value;
        private readonly FixtureOutput _output = new FixtureOutput();

        internal CounterGameModule(long initialValue) => _value = initialValue;

        internal static ModuleContract Contract { get; } = new ModuleContract(
            "counter-fixture",
            "1.0.0");

        public void Initialize(FixtureSnapshot? snapshot)
        {
            if (snapshot != null)
            {
                Restore(snapshot);
            }
        }

        public FixtureOutput Tick(in TickContext context, FixtureInput input)
        {
            _ = context;
            _value = unchecked(_value + input.Value);
            _output.Value = _value;
            return _output;
        }

        public FixtureSnapshot Snapshot() => new FixtureSnapshot { Value = _value };

        public void Restore(FixtureSnapshot snapshot) => _value = snapshot.Value;

        public void Dispose()
        {
        }
    }
}
