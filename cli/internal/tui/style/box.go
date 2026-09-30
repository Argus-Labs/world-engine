package style

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// BoxChrome is the horizontal space the border itself eats: "│ " + content +
// " │". Callers sizing a box against the terminal subtract this from the
// available columns.
const BoxChrome = 4

// minInnerWidth keeps a box renderable on an absurdly narrow terminal —
// below this the dash/padding arithmetic has nothing to work with.
const minInnerWidth = 8

//nolint:gochecknoglobals // read only, initialize once for performance (matches printer's style vars).
var (
	borderColor = lipgloss.NewStyle().Foreground(lipgloss.Color(Orange))
	titleColor  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(Yellow))
)

// Section is one titled region of a MultiSectionBox: a title and its body
// (may be multi-line, may already carry its own ANSI styling — e.g. a
// colored icon — which is left untouched).
type Section struct {
	Title string
	Body  string
}

// MultiSectionBox renders every section inside ONE continuous bordered box
// instead of one per section: the first title sits on the top border,
// later ones on a divider line (`├─ Title ──┤`) instead of their own
// top+bottom border. The box width fits the widest title/body line across
// all sections, so later sections can widen the whole box on the next
// render.
func MultiSectionBox(sections []Section) string {
	out, _ := MultiSectionBoxOpts(sections, BoxOpts{})
	return out
}

// BoxOpts tunes MultiSectionBoxOpts for a box rendered in pieces (see
// phasebox.Model, which commits finished sections to scrollback and keeps
// only the live one in the frame).
type BoxOpts struct {
	// MaxWidth clamps the box's total width, borders included. 0 means the
	// content decides. A box wider than the terminal gets hard-truncated by
	// bubbletea's renderer, which eats the right border off the long rows and
	// hides the tail of whatever error produced them.
	MaxWidth int
	// InnerWidth forces the content width instead of deriving it from the
	// widest line. Keeps every piece of a split box the same width, since
	// pieces already in scrollback can't be re-rendered.
	InnerWidth int
	// Continued opens with a "├─ Title ─┤" divider instead of a "╭" corner,
	// because earlier sections were already printed above.
	Continued bool
	// Open omits the closing "╰──╯" because more sections follow below.
	Open bool
}

// MultiSectionBoxOpts renders sections per opts and reports the inner width
// it settled on, so a caller splitting one box across several renders can
// pass that width back in and keep the pieces aligned.
func MultiSectionBoxOpts(sections []Section, opts BoxOpts) (string, int) {
	if len(sections) == 0 {
		return "", opts.InnerWidth
	}

	innerWidth := opts.InnerWidth
	if innerWidth <= 0 {
		for _, s := range sections {
			for line := range strings.SplitSeq(s.Body, "\n") {
				if w := lipgloss.Width(line); w > innerWidth {
					innerWidth = w
				}
			}
			// Title needs room for " Title " (its own 2 cols of padding) plus at
			// least one dash on either side within the border line — see
			// borderLine's width contract.
			if w := lipgloss.Width(s.Title) + 2; w > innerWidth {
				innerWidth = w
			}
		}
	}
	if opts.MaxWidth > 0 {
		innerWidth = min(innerWidth, opts.MaxWidth-BoxChrome)
	}
	innerWidth = max(innerWidth, minInnerWidth)
	dashSpan := innerWidth + 2 // +2: the 1-space padding either side of content

	lines := make([]string, 0, len(sections)+2)
	for i, s := range sections {
		switch {
		case i == 0 && !opts.Continued:
			lines = append(lines, borderLine("╭", "╮", s.Title, dashSpan))
		default:
			lines = append(lines, borderLine("├", "┤", s.Title, dashSpan))
		}
		for line := range strings.SplitSeq(s.Body, "\n") {
			lines = append(lines, contentLine(line, innerWidth))
		}
	}
	if !opts.Open {
		lines = append(lines, borderColor.Render("╰"+strings.Repeat("─", dashSpan)+"╯"))
	}

	return strings.Join(lines, "\n"), innerWidth
}

// borderLine renders a top/divider line: corner, dash, title (yellow),
// dashes to dashSpan, closing corner. Callers guarantee dashSpan >=
// len(" "+title+" ")+2 so the title always fits with a dash either side;
// falls back to a plain dashed line if title is empty or doesn't fit.
func borderLine(left, right, title string, dashSpan int) string {
	if title == "" {
		return borderColor.Render(left + strings.Repeat("─", dashSpan) + right)
	}
	const before = 1
	// Clamp the title to what's left after the leading dash and one trailing
	// dash, so a long title narrows itself instead of overflowing the box.
	if room := dashSpan - before - 1 - 2; room > 0 && lipgloss.Width(title) > room {
		title = ansi.Truncate(title, room, "…")
	}
	label := " " + title + " "
	after := dashSpan - before - lipgloss.Width(label)
	if after < 1 {
		return borderColor.Render(left + strings.Repeat("─", dashSpan) + right)
	}
	return borderColor.Render(left+strings.Repeat("─", before)) +
		titleColor.Render(label) +
		borderColor.Render(strings.Repeat("─", after)+right)
}

// contentLine pads line (which may already carry its own ANSI styling, e.g.
// a colored icon) to innerWidth visible columns and wraps it in colored "│"
// sides, with the same 1-space padding a lipgloss Padding(0,1) box uses.
func contentLine(line string, innerWidth int) string {
	// Truncate rather than overflow: an over-wide line is otherwise cut by
	// bubbletea's renderer at the screen edge, which drops this line's closing
	// "│" while shorter lines keep theirs — a visibly ragged box.
	if lipgloss.Width(line) > innerWidth {
		line = ansi.Truncate(line, innerWidth, "…")
	}
	pad := max(innerWidth-lipgloss.Width(line), 0)
	return borderColor.Render("│") + " " + line + strings.Repeat(" ", pad) + " " + borderColor.Render("│")
}
