//go:build windows

package worldscaffold

import (
	"os/exec"
	"syscall"
)

// createNoWindow is Windows' CREATE_NO_WINDOW process creation flag, which the
// syscall package does not export.
const createNoWindow = 0x08000000

// hideConsoleWindow makes cmd run without popping a console window.
//
// cardinal-editor is linked with -H windowsgui, so it owns no console and Windows
// allocates a fresh, visible one for every console-subsystem child — each `go`
// invocation flashes a terminal. CREATE_NO_WINDOW gives the child a console with
// no window instead. Grandchildren inherit it, which matters because the Go
// toolchain shells out to `git`; STARTF_USESHOWWINDOW/SW_HIDE would hide only the
// direct child and let those `git` windows through.
func hideConsoleWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
