package tableselect

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/suite"
)

type TableSelectTestSuite struct {
	suite.Suite

	ctx     context.Context
	columns []string
	rows    [][]string
}

func (suite *TableSelectTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.columns = []string{"Name", "Type"}
	suite.rows = [][]string{
		{"one", "alpha"},
		{"two", "beta"},
		{"three", "gamma"},
	}
}

func TestTableSelectSuite(t *testing.T) {
	suite.Run(t, new(TableSelectTestSuite))
}

func (suite *TableSelectTestSuite) TestInitialModel_SetsDefaults() {
	m := InitialModel(suite.ctx, "Select", suite.columns, suite.rows, 0)
	suite.Require().NotNil(m)
	suite.Equal(0, m.Cursor)
	suite.Len(m.Rows, 3)
}

func (suite *TableSelectTestSuite) TestDigitsMoveCursorAndBuffer() {
	m := InitialModel(suite.ctx, "Select", suite.columns, suite.rows, 0)

	ret, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = ret.(*Model)
	suite.Equal(1, m.Cursor)
	suite.Equal("2", m.NumericBuf)
}

func (suite *TableSelectTestSuite) TestArrowKeysUpdateCursorAndClearBuffer() {
	m := InitialModel(suite.ctx, "Select", suite.columns, suite.rows, 0)

	ret, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = ret.(*Model)
	suite.Equal(1, m.Cursor)
	suite.Empty(m.NumericBuf)

	ret, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = ret.(*Model)
	suite.Equal(0, m.Cursor)
	suite.Empty(m.NumericBuf)
}

func (suite *TableSelectTestSuite) TestBackspaceEditsBufferAndMovesCursor() {
	m := InitialModel(suite.ctx, "Select", suite.columns, suite.rows, 0)

	ret, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = ret.(*Model)
	ret, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = ret.(*Model)
	suite.Equal("12", m.NumericBuf)

	ret, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = ret.(*Model)
	suite.Equal("1", m.NumericBuf)
	suite.Equal(0, m.Cursor)
}

func (suite *TableSelectTestSuite) TestEscAborts() {
	m := InitialModel(suite.ctx, "Select", suite.columns, suite.rows, 0)
	ret, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = ret.(*Model)
	suite.True(m.Aborted)
}

func (suite *TableSelectTestSuite) TestEnterFinalizes() {
	m := InitialModel(suite.ctx, "Select", suite.columns, suite.rows, 0)
	ret, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = ret.(*Model)
	suite.True(m.finalized)
}

func (suite *TableSelectTestSuite) TestViewContainsPromptAndHint() {
	m := InitialModel(suite.ctx, "Pick item", suite.columns, suite.rows, 0)
	view := m.View()
	suite.Contains(view, "Pick item:")
	suite.Contains(view, "Type a number")
}

func (suite *TableSelectTestSuite) TestService_Run_NoItems() {
	idx, err := Run(suite.ctx, "Select", suite.columns, [][]string{}, 0)
	suite.Require().Error(err)
	suite.Equal(-1, idx)
}
