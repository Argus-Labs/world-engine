package printer

import (
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stripANSI removes all ANSI escape codes from a string.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// captureStdout captures [os.Stdout] output produced by fn and returns it as a string.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w //nolint:reassign // capturing stdout in tests

	fn()

	_ = w.Close()
	os.Stdout = orig //nolint:reassign // restore stdout
	buf, _ := io.ReadAll(r)
	_ = r.Close()
	return string(buf)
}

func TestSuccessVariants(t *testing.T) {
	out := captureStdout(t, func() { Success("ok") })
	assert.Contains(t, out, "ok")

	out = captureStdout(t, func() { Successln("line") })
	assert.Contains(t, out, "line")
	assert.True(t, strings.HasSuffix(out, "\n"))

	out = captureStdout(t, func() { Successf("num=%d", 7) })
	assert.Contains(t, out, "num=7")

	// Exercise newline-stripping path in printNewlineSafeStyledMessage
	out = captureStdout(t, func() { Successf("msg-with-nl\n") })
	assert.Contains(t, out, "msg-with-nl")
}

func TestErrorVariants(t *testing.T) {
	out := captureStdout(t, func() { Error("err") })
	assert.Contains(t, out, "err")

	out = captureStdout(t, func() { Errorln("errln") })
	assert.Contains(t, out, "errln")
	assert.True(t, strings.HasSuffix(out, "\n"))

	out = captureStdout(t, func() { Errorf("e=%s", "X") })
	assert.Contains(t, out, "e=X")
}

func TestInfoVariants(t *testing.T) {
	out := captureStdout(t, func() { Info("info") })
	assert.Contains(t, out, "info")

	out = captureStdout(t, func() { Infoln("infoln") })
	assert.Contains(t, out, "infoln")
	assert.True(t, strings.HasSuffix(out, "\n"))

	out = captureStdout(t, func() { Infof("i=%d", 3) })
	assert.Contains(t, out, "i=3")
}

func TestHeaderVariants(t *testing.T) {
	out := captureStdout(t, func() { Header("HEAD") })
	assert.Contains(t, stripANSI(out), "HEAD")

	out = captureStdout(t, func() { Headerln("HEADLN") })
	assert.Contains(t, stripANSI(out), "HEADLN")
	assert.True(t, strings.HasSuffix(out, "\n"))

	out = captureStdout(t, func() { Headerf("H=%s", "X") })
	assert.Contains(t, stripANSI(out), "H=X")
}

func TestNotificationVariants(t *testing.T) {
	out := captureStdout(t, func() { Notification("note") })
	assert.Contains(t, out, "note")

	out = captureStdout(t, func() { Notificationln("noteln") })
	assert.Contains(t, out, "noteln")
	assert.True(t, strings.HasSuffix(out, "\n"))

	out = captureStdout(t, func() { Notificationf("n=%d", 5) })
	assert.Contains(t, out, "n=5")
}

func TestNewLine(t *testing.T) {
	// zero should print nothing
	out := captureStdout(t, func() { NewLine(0) })
	assert.Empty(t, out)

	// positive should print that many newlines
	out = captureStdout(t, func() { NewLine(3) })
	assert.Equal(t, "\n\n\n", out)
}

func TestCursorAndClear(t *testing.T) {
	// We can't assert exact escape sequences cross-platform, but should output something
	out := captureStdout(t, func() { MoveCursorUp(2) })
	assert.NotEmpty(t, out)
	assert.Contains(t, out, "\x1b[")

	out = captureStdout(t, func() { MoveCursorRight(4) })
	assert.NotEmpty(t, out)
	assert.Contains(t, out, "\x1b[")

	out = captureStdout(t, func() { ClearToEndOfLine() })
	assert.NotEmpty(t, out)
	assert.Contains(t, out, "\x1b[")
}

func TestSectionDivider(t *testing.T) {
	// default length fallback to 1
	out := captureStdout(t, func() { SectionDivider("-", 0) })
	assert.Equal(t, "-\n", out)

	// explicit length
	out = captureStdout(t, func() { SectionDivider("*", 5) })
	assert.Equal(t, "*****\n", out)
}
