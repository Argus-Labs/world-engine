package logtail

import (
	"context"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/worldstatus"
)

// mustQuit invokes cmd (which must be non-nil) and asserts it resolves to a
// tea.QuitMsg — the only way to observe "this Update call asked the Program
// to quit" from outside the bubbletea runtime.
func mustQuit(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	require.NotNil(t, cmd, "expected a quit Cmd")
	_, ok := cmd().(tea.QuitMsg)
	require.True(t, ok, "expected cmd to resolve to tea.QuitMsg")
}

func TestTailModel_Enter_Quits(t *testing.T) {
	t.Parallel()

	m := New(context.Background(), true, nil)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mustQuit(t, cmd)
}

func TestTailModel_CtrlC_QuitsAndMarksCanceled(t *testing.T) {
	t.Parallel()

	m := New(context.Background(), true, nil)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	mustQuit(t, cmd)

	out, ok := next.(*Model)
	require.True(t, ok)
	require.True(t, out.canceled)
}

func TestTailModel_ReloadKey_WhenEnabled_QuitsAndMarksReload(t *testing.T) {
	t.Parallel()

	m := New(context.Background(), true, nil)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(DefaultReloadKeys().Reload.Keys()[0])})
	mustQuit(t, cmd)

	out, ok := next.(*Model)
	require.True(t, ok)
	require.True(t, out.reload)
	require.False(t, out.canceled)
}

func TestTailModel_ReloadKey_WhenDisabled_IsIgnored(t *testing.T) {
	t.Parallel()

	m := New(context.Background(), false, nil)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(DefaultReloadKeys().Reload.Keys()[0])})
	require.Nil(t, cmd, "reload hotkey must be a no-op when onReload is nil")

	out, ok := next.(*Model)
	require.True(t, ok)
	require.False(t, out.reload)
}

func TestTailModel_UnknownKey_IsIgnored(t *testing.T) {
	t.Parallel()

	m := New(context.Background(), true, nil)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	require.Nil(t, cmd)

	out, ok := next.(*Model)
	require.True(t, ok)
	require.False(t, out.reload)
	require.False(t, out.canceled)
}

// TestTailModel_CtxDone_QuitsRegardlessOfMessage guards the property
// TailLogsUntilEnterOrReload depends on for external cancellation (SIGTERM):
// once ctx is done, Update quits on the very next message it sees,
// regardless of what that message is.
func TestTailModel_CtxDone_QuitsRegardlessOfMessage(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	m := New(ctx, true, nil)
	_, cmd := m.Update(LogLineMsg{Line: "irrelevant"})
	mustQuit(t, cmd)
}

func TestTailModel_LogLine_ReturnsNonNilCmd(t *testing.T) {
	t.Parallel()

	m := New(context.Background(), true, nil)
	_, cmd := m.Update(LogLineMsg(worldstatus.LogLine{InstanceName: "gameplay-2", Line: "hello"}))
	require.NotNil(t, cmd, "a log line must schedule a print")
}

func TestTailModel_StreamEnded_ReturnsNonNilCmd(t *testing.T) {
	t.Parallel()

	m := New(context.Background(), true, nil)
	_, cmd := m.Update(StreamEndedMsg{})
	require.NotNil(t, cmd, "stream end must schedule the closing banner")
}

// The picker and the tail view must bind the same keys; sharing ReloadKeys is
// what guarantees it, so assert the bindings carry both key and help text.
func TestDefaultReloadKeys_CarryKeysAndHelp(t *testing.T) {
	t.Parallel()
	keys := DefaultReloadKeys()

	assert.Equal(t, []string{"r"}, keys.Reload.Keys())
	assert.Equal(t, "reload", keys.Reload.Help().Desc)
	assert.Equal(t, []string{"ctrl+r"}, keys.PurgeReload.Keys())
	assert.Equal(t, "purge & reload", keys.PurgeReload.Help().Desc)
}

// A session with no reload hook must neither advertise the keys nor act on them.
func TestReloadKeys_SetEnabled(t *testing.T) {
	t.Parallel()
	keys := DefaultReloadKeys().WithEnabled(false)

	assert.False(t, keys.Reload.Enabled())
	assert.False(t, keys.PurgeReload.Enabled())
}

// '?' expands the legend without ending the tail session.
func TestTailModel_HelpKey_TogglesWithoutQuitting(t *testing.T) {
	t.Parallel()
	m := New(context.Background(), true, stubExtras())
	require.Contains(t, plain(m.View()), "? more")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	require.Nil(t, cmd, "help must not quit the tail view")

	out, ok := next.(*Model)
	require.True(t, ok)
	assert.Contains(t, plain(out.View()), "? less")
	assert.False(t, out.reload)
}

// stubExtras supplies four in-place actions without touching a cluster, named
// the way the log viewer names them.
func stubExtras() []ExtraAction {
	action := func(digit, label string) ExtraAction {
		return ExtraAction{
			Binding: key.NewBinding(key.WithKeys(digit), key.WithHelp(digit, label)),
			Run:     func(context.Context) (string, error) { return label + "ed", nil },
		}
	}
	return []ExtraAction{
		action("1", "resum"), action("2", "paus"), action("3", "step"), action("4", "reset"),
	}
}

// Extra actions run without closing the tail session.
func TestTailModel_DebugKey_RunsWithoutQuitting(t *testing.T) {
	t.Parallel()
	m := New(context.Background(), true, stubExtras())

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	require.NotNil(t, cmd, "pause must produce a command")

	out, ok := next.(*Model)
	require.True(t, ok)
	assert.False(t, out.reload)
	assert.False(t, out.canceled)

	// The command carries the hook's result back as a message.
	msg, ok := cmd().(actionResultMsg)
	require.True(t, ok)
	require.NoError(t, msg.err)
	assert.Equal(t, "paused", msg.text)
}

// Without hooks the digits are inert, so a tail spanning every shard cannot
// pause one of them by accident.
func TestTailModel_DebugKeys_DisabledWithoutHooks(t *testing.T) {
	t.Parallel()
	m := New(context.Background(), true, nil)

	for _, digit := range []string{"1", "2", "3", "4"} {
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(digit)})
		assert.Nil(t, cmd, digit)
	}
}

// A failing hook reports rather than ending the session.
func TestTailModel_DebugResult_PrintsError(t *testing.T) {
	t.Parallel()
	m := New(context.Background(), true, stubExtras())

	next, cmd := m.Update(actionResultMsg{err: eris.New("world is not paused")})
	assert.NotNil(t, cmd)
	out, ok := next.(*Model)
	require.True(t, ok)
	assert.False(t, out.canceled)
}

// Extra actions appear only in the expanded legend.
func TestTailModel_HelpLines(t *testing.T) {
	t.Parallel()
	m := New(context.Background(), true, stubExtras())

	short := plain(m.legend.View(m.primaryBindings(), secondaryBindings(m.extras)))
	assert.Contains(t, short, "enter menu")
	assert.Contains(t, short, "r reload")
	assert.Contains(t, short, "ctrl+r purge & reload")
	assert.Contains(t, short, "? more")
	assert.NotContains(t, short, "quit", "ctrl+c is a terminal convention, not ours to teach")
	assert.NotContains(t, short, "resume")

	require.True(t, m.legend.HandleToggle(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")}, true))
	full := plain(m.legend.View(m.primaryBindings(), secondaryBindings(m.extras)))
	assert.Contains(t, full, "1 resum")
	assert.Contains(t, full, "2 paus")
	assert.Contains(t, full, "3 step")
	assert.Contains(t, full, "4 reset")
	assert.Contains(t, full, "? less")
}

// plain strips the colour lipgloss adds when the renderer thinks it has a
// terminal. Asserting on the raw view passes locally, where colour is off, and
// fails in CI, where it is on — so every legend assertion goes through here.
func plain(s string) string { return ansi.Strip(s) }
