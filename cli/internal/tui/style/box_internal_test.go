package style

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

func TestMultiSectionBox_FirstTitleOnTopBorder(t *testing.T) {
	t.Parallel()

	got := MultiSectionBox([]Section{
		{Title: "Cluster", Body: "hello world, this is a reasonably long body line"},
	})
	lines := strings.Split(got, "\n")
	require.NotEmpty(t, lines)

	// Regression: colorizing wraps border segments in SGR escape sequences —
	// stripped of color codes, the corner + title must be intact (a naive
	// rune-index splice into an already-colored line instead landed inside
	// the escape code, eating "W" off "WORLD ENGINE").
	require.True(t, strings.HasPrefix(stripANSI(lines[0]), "╭─ Cluster "),
		"stripped of color codes, the top border should read corner + title intact")
}

func TestMultiSectionBox_LaterTitlesOnDividerLines(t *testing.T) {
	t.Parallel()

	got := MultiSectionBox([]Section{
		{Title: "Image Pull", Body: "✓ 2 image(s) pulled (1s)"},
		{Title: "Build", Body: "✓ 2 image(s) built (1s)"},
	})
	lines := strings.Split(stripANSI(got), "\n")
	require.Len(t, lines, 5, "top border, pull line, divider, build line, bottom border")
	require.True(t, strings.HasPrefix(lines[0], "╭─ Image Pull "))
	require.True(t, strings.HasPrefix(lines[2], "├─ Build "), "later sections split with a ├─ divider, not a new ╭ box")
	require.True(t, strings.HasSuffix(lines[2], "┤"))
	require.True(t, strings.HasPrefix(lines[4], "╰"), "only one bottom border for the whole multi-section box")
}

func TestMultiSectionBox_TitleIsYellowBorderIsOrange(t *testing.T) {
	t.Parallel()

	got := MultiSectionBox([]Section{{Title: "Cluster", Body: "hi there"}})
	titleRendered := titleColor.Render(" Cluster ")
	require.Contains(t, got, titleRendered, "title text should be rendered in the yellow title style")
}

func TestMultiSectionBox_WidensToFitWidestSection(t *testing.T) {
	t.Parallel()

	// The first section's body is short; a later section's body is much
	// wider — the whole box (including the first section's top border and
	// content padding) must widen to fit it, since it's all one box.
	got := MultiSectionBox([]Section{
		{Title: "Image Pull", Body: "x"},
		{Title: "Build", Body: "a rather long line that forces the whole shared box to widen"},
	})
	lines := strings.Split(stripANSI(got), "\n")
	width := len([]rune(lines[0]))
	for _, l := range lines {
		require.Equal(t, width, len([]rune(l)), "every line (border, divider, content) must share one box width")
	}
}

func TestMultiSectionBox_LeavesEmbeddedContentStylingUntouched(t *testing.T) {
	t.Parallel()

	// A row line's inner content (e.g. a colored icon) must survive
	// untouched — only the leading/trailing "│" get colorized.
	styled := "\x1b[38;5;9m✗\x1b[0m failed row"
	got := MultiSectionBox([]Section{{Title: "Section", Body: styled}})
	require.Contains(t, got, styled, "embedded ANSI styling inside a row must not be stripped or altered")
}

func TestMultiSectionBox_Empty(t *testing.T) {
	t.Parallel()

	require.Equal(t, "", MultiSectionBox(nil))
}

func TestMultiSectionBoxOpts_ClampsToMaxWidth(t *testing.T) {
	t.Parallel()

	// A long cluster failure is fed in raw at ~10 call sites; unclamped it makes
	// every line wider than the screen, and bubbletea's renderer then cuts the
	// closing "│" off the long rows while short rows keep theirs.
	long := "ensure cluster: " + strings.Repeat("failure detail ", 20)
	got, inner := MultiSectionBoxOpts([]Section{{Title: "Cluster", Body: long}}, BoxOpts{MaxWidth: 80})

	require.Equal(t, 80-BoxChrome, inner, "inner width should fill the terminal minus the border chrome")
	for line := range strings.SplitSeq(got, "\n") {
		require.LessOrEqual(t, lipgloss.Width(line), 80, "no line may exceed MaxWidth")
	}
	require.Contains(t, stripANSI(got), "…", "over-wide content should be truncated, not overflowed")
}

func TestMultiSectionBoxOpts_ContinuedStartsWithDividerAndOpenOmitsBottom(t *testing.T) {
	t.Parallel()

	got, _ := MultiSectionBoxOpts(
		[]Section{{Title: "Build", Body: "✓ 2 image(s) built (1s)"}},
		BoxOpts{Continued: true, Open: true},
	)
	lines := strings.Split(stripANSI(got), "\n")

	require.True(t, strings.HasPrefix(lines[0], "├─ Build "),
		"a continued piece joins what was already printed above instead of opening a new box")
	require.False(t, strings.HasPrefix(lines[len(lines)-1], "╰"),
		"an open piece leaves the closing border to the live piece below it")
}

func TestMultiSectionBoxOpts_NarrowTerminalDoesNotPanic(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		MultiSectionBoxOpts(
			[]Section{{Title: "A Very Long Section Title", Body: "some content here"}},
			BoxOpts{MaxWidth: 10},
		)
	}, "negative dash/padding counts would panic strings.Repeat")
}
