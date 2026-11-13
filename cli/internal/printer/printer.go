//nolint:forbidigo // Printer is used for customer friendly output to terminal
package printer

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/guumaster/logsymbols"
	"github.com/muesli/termenv"
)

//nolint:gochecknoglobals // read only, initialize objects once for performance.
var (
	successStyle      = lipgloss.NewStyle().Bold(true)
	errorStyle        = lipgloss.NewStyle().Bold(true)
	headerStyle       = lipgloss.NewStyle().Bold(true).Underline(true)
	notificationStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("178")) // Bright yellow, good for notifications
)

func Successf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	printNewlineSafeStyledMessage(string(logsymbols.Success)+" "+msg, successStyle)
}

// Errorln prints msg as one ✖ entry, later lines indented under it. Styled
// line by line: lipgloss pads a multi-line render to its widest line.
func Errorln(msg string) {
	prefix := string(logsymbols.Error) + " "
	for line := range strings.SplitSeq(msg, "\n") {
		fmt.Println(errorStyle.Render(prefix + line))
		prefix = "  "
	}
}

// Errorf is Errorln with a format; a trailing newline in it is optional.
func Errorf(format string, args ...any) {
	Errorln(strings.TrimSuffix(fmt.Sprintf(format, args...), "\n"))
}

func Infoln(msg string) {
	fmt.Println(msg)
}

func Infof(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Print(msg)
}

func Headerln(msg string) {
	fmt.Println(headerStyle.Render(msg))
}

func Notificationf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	printNewlineSafeStyledMessage(msg, notificationStyle)
}

func Notificationln(msg string) {
	fmt.Println(notificationStyle.Render(msg))
}

func NewLine(numberOfLines int) {
	if numberOfLines <= 0 {
		return
	}
	fmt.Print(strings.Repeat("\n", numberOfLines))
}

func MoveCursorUp(numberOfLines int) {
	output := termenv.NewOutput(os.Stdout)
	output.CursorUp(numberOfLines)
}

func ClearToEndOfLine() {
	output := termenv.NewOutput(os.Stdout)
	output.ClearLineRight()
}

// SectionDivider prints a divider line of a given symbol and length.
// Default length is 1.
func SectionDivider(symbol string, length int) {
	if length <= 0 {
		length = 1
	}
	fmt.Println(strings.Repeat(symbol, length))
}

func printNewlineSafeStyledMessage(msg string, style lipgloss.Style) {
	if before, ok := strings.CutSuffix(msg, "\n"); ok {
		msg = before
		styledMsg := style.Render(msg)
		fmt.Println(styledMsg)
	} else {
		fmt.Print(style.Render(msg))
	}
}
