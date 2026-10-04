package tablepageselect

import (
	"context"
	"fmt"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/suite"
)

type TablePageSelectTestSuite struct {
	suite.Suite

	ctx     context.Context
	columns []string
	rows    [][]string
}

func (suite *TablePageSelectTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.columns = []string{"Name", "Type"}
	// Build 15 rows so we have two pages (10 + 5).
	suite.rows = make([][]string, 15)
	for i := range 15 {
		suite.rows[i] = []string{fmt.Sprintf("item-%02d", i+1), "alpha"}
	}
}

func TestTablePageSelectSuite(t *testing.T) {
	suite.Run(t, new(TablePageSelectTestSuite))
}

func (suite *TablePageSelectTestSuite) TestInitialModel_SetsDefaults() {
	m := InitialModel(suite.ctx, "Select", suite.columns, suite.rows, 0)
	suite.Require().NotNil(m)
	suite.Equal(0, m.Cursor)
	suite.Len(m.Rows, 15)
}

func (suite *TablePageSelectTestSuite) TestDownAcrossPageMovesToNextPageTop() {
	m := InitialModel(suite.ctx, "Select", suite.columns, suite.rows, 9) // select 10th item
	ret, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = ret.(*Model)
	suite.Equal(10, m.Cursor) // first row of second page (11th item)
	// moving back up should return to previous page's last row
	ret, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = ret.(*Model)
	suite.Equal(9, m.Cursor)
}

func (suite *TablePageSelectTestSuite) TestLeftRightSkipPagesPreservingOffset() {
	m := InitialModel(suite.ctx, "Select", suite.columns, suite.rows, 2)
	// Right: jump +10 => index 12
	ret, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = ret.(*Model)
	suite.Equal(12, m.Cursor)
	// Left: jump -10 => index 2
	ret, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = ret.(*Model)
	suite.Equal(2, m.Cursor)
	// Right at near end should clamp to last index
	m = InitialModel(suite.ctx, "Select", suite.columns, suite.rows, 8) // 9th item
	ret, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = ret.(*Model)
	suite.Equal(14, m.Cursor) // last item (15th)
}

func (suite *TablePageSelectTestSuite) TestDigitsMoveCursorAndBufferAcrossPages() {
	m := InitialModel(suite.ctx, "Select", suite.columns, suite.rows, 0)
	ret, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = ret.(*Model)
	ret, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = ret.(*Model)
	suite.Equal(11, m.Cursor) // 12th item (index 11)
	suite.Equal("12", m.NumericBuf)
	// backspace edits buffer and moves cursor
	ret, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = ret.(*Model)
	suite.Equal("1", m.NumericBuf)
	suite.Equal(0, m.Cursor) // 1st item
}

func (suite *TablePageSelectTestSuite) TestEscAborts() {
	m := InitialModel(suite.ctx, "Select", suite.columns, suite.rows, 0)
	ret, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = ret.(*Model)
	suite.True(m.Aborted)
}

func (suite *TablePageSelectTestSuite) TestCtrlCAborts() {
	m := InitialModel(suite.ctx, "Select", suite.columns, suite.rows, 0)
	ret, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = ret.(*Model)
	suite.True(m.Canceled)
}

func (suite *TablePageSelectTestSuite) TestEnterFinalizes() {
	m := InitialModel(suite.ctx, "Select", suite.columns, suite.rows, 0)
	ret, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = ret.(*Model)
	suite.True(m.finalized)
}

func (suite *TablePageSelectTestSuite) TestViewContainsPromptAndHint() {
	m := InitialModel(suite.ctx, "Pick item", suite.columns, suite.rows, 0)
	view := plain(m.View())
	suite.Contains(view, "Pick item:")
	suite.Contains(view, "nav")
	suite.Contains(view, "enter select")
	suite.Contains(view, "esc quit")
}

func (suite *TablePageSelectTestSuite) TestNoExtraKeys_HidesHelpToggle() {
	m := InitialModel(suite.ctx, "Pick item", suite.columns, suite.rows, 0)
	suite.NotContains(plain(m.View()), "more")

	ret, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = ret.(*Model)
	suite.NotContains(plain(m.View()), "less")
}

func (suite *TablePageSelectTestSuite) TestSecondaryExtraKey() {
	m := InitialModel(suite.ctx, "Pick item", suite.columns, suite.rows, 0)
	m.ExtraKeys = []ExtraKey{
		{
			Binding: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reload")),
			Action:  "reload",
		},
		{
			Binding:   key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "purge & reload")),
			Action:    "reload-purge",
			Secondary: true,
		},
	}

	short := plain(m.View())
	suite.Contains(short, "r reload")
	suite.NotContains(short, "purge & reload")

	ret, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = ret.(*Model)
	suite.Contains(plain(m.View()), "purge & reload")
}

func (suite *TablePageSelectTestSuite) TestExtraKeyReturnsAction() {
	m := InitialModel(suite.ctx, "Pick item", suite.columns, suite.rows, 0)
	m.ExtraKeys = []ExtraKey{{
		Binding: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reload")),
		Action:  "reload",
	}}

	ret, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = ret.(*Model)
	suite.NotNil(cmd)
	suite.True(m.finalized)
	suite.Equal("reload", m.HotkeyAction)
}

func (suite *TablePageSelectTestSuite) TestHelpKeyTogglesFullHelp() {
	m := InitialModel(suite.ctx, "Pick item", suite.columns, suite.rows, 0)
	m.ExtraKeys = []ExtraKey{{
		Binding: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reload")),
		Action:  "reload",
	}}
	suite.Contains(plain(m.View()), "? more")

	// "/" is the shift-free alias for "?".
	ret, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = ret.(*Model)
	// Expanded help pads the key and label apart.
	suite.Contains(plain(m.View()), "less")

	ret, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = ret.(*Model)
	suite.Contains(plain(m.View()), "? more")
}

func (suite *TablePageSelectTestSuite) TestHelpKeyIsNotAnAction() {
	m := InitialModel(suite.ctx, "Pick item", suite.columns, suite.rows, 0)
	m.ExtraKeys = []ExtraKey{{
		Binding: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reload")),
		Action:  "reload",
	}}
	ret, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = ret.(*Model)
	suite.False(m.finalized)
	suite.Empty(m.HotkeyAction)
}

// plain makes assertions independent of terminal color support.
func plain(s string) string { return ansi.Strip(s) }
