package debugger

import (
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// freshCLI returns a Cmd with every subcommand pre-allocated so kong.Parse can
// populate each command's Instances field and the test can read it back
// directly. Kong only allocates the branch along the selected path, so
// pre-allocating keeps field access uniform across subcommands.
func freshCLI() *Cmd {
	return &Cmd{
		Pause:  &PauseCmd{},
		Resume: &ResumeCmd{},
		Step:   &StepCmd{},
		Reset:  &ResetCmd{},
	}
}

// TestDebuggerSubcommandFlagAliases verifies that the canonical --shards flag
// and its --shard-id / --instances aliases are all accepted by each of the
// four debugger subcommands and that the value binds to the same Instances
// field regardless of which spelling is used. This is the regression guard for
// the kong-parse layer that the integration suite (//go:build integration)
// exercises end-to-end and that helpers_internal_test.go bypasses by calling
// runDebugAction directly.
func TestDebuggerSubcommandFlagAliases(t *testing.T) {
	t.Parallel()

	subs := []struct {
		name string
		got  func(c *Cmd) []string
	}{
		{"pause", func(c *Cmd) []string { return c.Pause.Instances }},
		{"resume", func(c *Cmd) []string { return c.Resume.Instances }},
		{"step", func(c *Cmd) []string { return c.Step.Instances }},
		{"reset", func(c *Cmd) []string { return c.Reset.Instances }},
	}

	flags := []string{"shards", "shard-id", "instances"}

	for _, sub := range subs {
		for _, flag := range flags {
			t.Run(sub.name+"_"+flag, func(t *testing.T) {
				t.Parallel()

				cli := freshCLI()
				p, err := kong.New(cli)
				require.NoError(t, err)

				_, err = p.Parse([]string{sub.name, "--" + flag, "game"})
				require.NoError(t, err, "flag --%s should be accepted by 'debug %s'", flag, sub.name)
				assert.Equal(t, []string{"game"}, sub.got(cli),
					"flag --%s should bind its value to %s.Instances", flag, sub.name)
			})
		}
	}
}

// TestDebuggerSubcommandFlagAliasesAccumulate verifies the Instances slice
// accumulates across repeatable occurrences, comma-separated values inside a
// single occurrence, and mixed aliases on a single command invocation —
// matching the help-text contract ("Repeatable or comma-separated").
func TestDebuggerSubcommandFlagAliasesAccumulate(t *testing.T) {
	t.Parallel()

	t.Run("repeatable across occurrences", func(t *testing.T) {
		t.Parallel()

		cli := freshCLI()
		p, err := kong.New(cli)
		require.NoError(t, err)

		_, err = p.Parse([]string{"pause", "--shards", "game", "--shards", "meta"})
		require.NoError(t, err)
		assert.Equal(t, []string{"game", "meta"}, cli.Pause.Instances)
	})

	t.Run("comma separated in one occurrence", func(t *testing.T) {
		t.Parallel()

		cli := freshCLI()
		p, err := kong.New(cli)
		require.NoError(t, err)

		_, err = p.Parse([]string{"pause", "--shards", "game,meta"})
		require.NoError(t, err)
		assert.Equal(t, []string{"game", "meta"}, cli.Pause.Instances)
	})

	t.Run("aliases mix into the same field", func(t *testing.T) {
		t.Parallel()

		cli := freshCLI()
		p, err := kong.New(cli)
		require.NoError(t, err)

		_, err = p.Parse([]string{"resume", "--shard-id", "game", "--instances", "meta"})
		require.NoError(t, err)
		assert.Equal(t, []string{"game", "meta"}, cli.Resume.Instances)
	})
}

// TestDebuggerSubcommandRejectsUnknownFlag confirms the aliasing did not
// accidentally disable kong's flag validation: a completely unknown flag is
// still rejected with a parse error for every debugger subcommand.
func TestDebuggerSubcommandRejectsUnknownFlag(t *testing.T) {
	t.Parallel()

	for _, sub := range []string{"pause", "resume", "step", "reset"} {
		t.Run(sub, func(t *testing.T) {
			t.Parallel()

			cli := freshCLI()
			p, err := kong.New(cli)
			require.NoError(t, err)

			_, err = p.Parse([]string{sub, "--bogus-flag", "game"})
			require.Error(t, err, "unknown flag should be rejected by 'debug %s'", sub)
		})
	}
}
