//go:build windows

package netcfg

import (
	"os/exec"
	"syscall"
)

// createNoWindow runs the child process without allocating a console, so netsh
// invocations from the GUI app don't flash a command window on screen.
const createNoWindow = 0x08000000

func hideWindow(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
