// Package logtail displays streamed logs and handles tail-session input.
package logtail

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/rotisserie/eris"

	tuikeys "github.com/argus-labs/world-engine/cli/internal/tui/kit/keys"
	"github.com/argus-labs/world-engine/cli/internal/tui/style"
	"github.com/argus-labs/world-engine/cli/pkg/cluster"
)

type LogLineMsg cluster.LogLine

// StreamEndedMsg reports that the log stream ended.
type StreamEndedMsg struct{ Err error }

type actionResultMsg struct {
	text string
	err  error
}

type keyMap struct {
	Menu   key.Binding
	Quit   key.Binding
	Reload ReloadKeys
}

func newKeyMap(canReload bool) keyMap {
	keys := keyMap{
		Menu:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "menu")),
		Quit:   key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
		Reload: DefaultReloadKeys(),
	}
	keys.Reload = keys.Reload.WithEnabled(canReload)
	return keys
}

// Model displays streamed logs and handles tail-session input.
type Model struct {
	ctx    context.Context
	keys   keyMap
	legend tuikeys.Legend

	labelColor func(string) string

	extras []ExtraAction
	width  int

	reload   bool
	purge    bool
	canceled bool
}

// New creates a log-tail model.
func New(ctx context.Context, canReload bool, extras []ExtraAction) *Model {
	return &Model{
		ctx:        ctx,
		keys:       newKeyMap(canReload),
		legend:     tuikeys.NewLegend(),
		extras:     extras,
		labelColor: newLabelColorAssigner(),
	}
}

// Canceled reports that the user interrupted with Ctrl+C.
func (m *Model) Canceled() bool { return m.canceled }

// Reload reports whether the user requested a reload.
func (m *Model) Reload() bool { return m.reload }

// Purge reports whether the requested reload should purge state first.
func (m *Model) Purge() bool { return m.purge }

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	select {
	case <-m.ctx.Done():
		return m, tea.Quit
	default:
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.legend.SetWidth(msg.Width)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case LogLineMsg:
		line := cluster.LogLine(msg)
		label := line.InstanceName
		if label == "" {
			label = line.ShardID
		}
		if label == "" {
			label = line.PodName
		}
		return m, tea.Println(m.fit(formatShardLine(label, m.labelColor(label), line.Line)))

	case actionResultMsg:
		if msg.err != nil {
			return m, tea.Println(m.fit(style.ForegroundPrint(
				fmt.Sprintf("✖ %v", msg.err), style.Orange)))
		}
		return m, tea.Println(m.fit(style.ForegroundPrint("✔ "+msg.text, style.Yellow)))

	case StreamEndedMsg:
		var cmds []tea.Cmd
		if msg.Err != nil && !eris.Is(msg.Err, context.Canceled) {
			cmds = append(cmds, tea.Println(fmt.Sprintf("Error tailing shard logs: %v", msg.Err)))
		}
		cmds = append(cmds, tea.Println("All log streams ended. Press ENTER to return."))
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// primaryBindings omits Ctrl+C because it is a terminal convention.
func (m *Model) primaryBindings() []key.Binding {
	return []key.Binding{
		m.keys.Menu, m.keys.Reload.Reload, m.keys.Reload.PurgeReload,
	}
}

// handleKey exits for session keys; extra actions run in place.
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Menu):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Quit):
		m.canceled = true
		return m, tea.Quit
	case key.Matches(msg, m.keys.Reload.Reload):
		m.reload = true
		return m, tea.Quit
	case key.Matches(msg, m.keys.Reload.PurgeReload):
		m.reload = true
		m.purge = true
		return m, tea.Quit
	case m.legend.HandleToggle(msg, len(m.extras) > 0):
		return m, nil
	}

	// Built-in keys take precedence over caller-supplied keys.
	for _, extra := range m.extras {
		if key.Matches(msg, extra.Binding) {
			return m, m.run(extra.Run)
		}
	}
	return m, nil
}

// fit wraps output before the terminal can overwrite the legend.
func (m *Model) fit(line string) string {
	if m.width <= 0 {
		return line
	}
	return ansi.Hardwrap(line, m.width, false)
}

// run executes an extra action without blocking Update.
func (m *Model) run(action func(context.Context) (string, error)) tea.Cmd {
	if action == nil {
		return nil
	}
	return func() tea.Msg {
		text, err := action(m.ctx)
		return actionResultMsg{text: text, err: err}
	}
}

func (m *Model) View() string {
	legend := m.legend.View(m.primaryBindings(), secondaryBindings(m.extras))
	return "\n" + style.ForegroundPrint(legend, style.Yellow) + "\n"
}

func paletteColor(idx int) string {
	colors := [...]string{
		"#00FF00", // Green
		"#0000FF", // Blue
		"#00FFFF", // Cyan
		"#FF00FF", // Magenta
		"#FFA500", // Orange
		"#800080", // Purple
		"#FFC0CB", // Pink
		"#87CEEB", // Sky Blue
		"#32CD32", // Lime Green
	}
	return colors[idx%len(colors)]
}

func newLabelColorAssigner() func(label string) string {
	assigned := map[string]string{}
	next := 0
	return func(label string) string {
		if c, ok := assigned[label]; ok {
			return c
		}
		c := paletteColor(next)
		assigned[label] = c
		next++
		return c
	}
}
