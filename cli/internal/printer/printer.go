//nolint:forbidigo // Printer is used for customer friendly output to terminal
package printer

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/guumaster/logsymbols"
	"github.com/muesli/termenv"

	"github.com/argus-labs/world-engine/cli/internal/tui/style"
)

//nolint:gochecknoglobals // read only, initialize objects once for performance.
var (
	successStyle      = lipgloss.NewStyle().Bold(true)
	errorStyle        = lipgloss.NewStyle().Bold(true)
	headerStyle       = lipgloss.NewStyle().Bold(true).Underline(true)
	notificationStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("178")) // Bright yellow, good for notifications
)

func Success(msg string) {
	printNewlineSafeStyledMessage(string(logsymbols.Success)+" "+msg, successStyle)
}

func Successln(msg string) {
	fmt.Println(successStyle.Render(string(logsymbols.Success) + " " + msg))
}

func Successf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	printNewlineSafeStyledMessage(string(logsymbols.Success)+" "+msg, successStyle)
}

func Error(msg string) {
	printNewlineSafeStyledMessage(string(logsymbols.Error)+" "+msg, errorStyle)
}

func Errorln(msg string) {
	fmt.Println(errorStyle.Render(string(logsymbols.Error) + " " + msg))
}

func Errorf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	printNewlineSafeStyledMessage(string(logsymbols.Error)+" "+msg, errorStyle)
}

func Info(msg string) {
	fmt.Print(msg)
}

func Infoln(msg string) {
	fmt.Println(msg)
}

func Infof(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Print(msg)
}

// Step prints a one-line completion message in the same visual format the
// docker-build multispinner uses for its "done" rows:
//
//	✓  <state>  <kind>  <name>
//
// state is rendered light-blue, kind is light-magenta, name is green; the
// leading ✓ is orange + bold (style.TickIcon). Use for one-shot ops that
// finish synchronously (cluster ready, image imported, shards rolled, etc.)
// so the cluster bring-up + build + reload sections all look cohesive.
//
// For parallel build-style progress (multi-image), use the multispinner.
func Step(state, kind, name string) {
	fmt.Println(
		style.TickIcon.Render() + " " +
			style.ForegroundPrint(state, "12") + " " +
			style.ForegroundPrint(kind, "13") + " " +
			style.ForegroundPrint(name, "2"),
	)
}

func Header(msg string) {
	fmt.Print(headerStyle.Render(msg))
}

func Headerln(msg string) {
	fmt.Println(headerStyle.Render(msg))
}

func Headerf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	printNewlineSafeStyledMessage(msg, headerStyle)
}

func Notification(msg string) {
	printNewlineSafeStyledMessage(msg, notificationStyle)
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

func MoveCursorRight(numberOfCells int) {
	output := termenv.NewOutput(os.Stdout)
	output.CursorForward(numberOfCells)
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
