package netcfg

import (
	"fmt"
	"os/exec"
)

// runAll executes a sequence of commands, stopping at the first failure. Shared
// by the Linux (iproute2) and Windows (netsh) routers.
func runAll(cmds [][]string) error {
	for _, cmd := range cmds {
		c := exec.Command(cmd[0], cmd[1:]...)
		hideWindow(c) // suppress flashing console windows on Windows (no-op elsewhere)
		if out, err := c.CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %w: %s", cmd, err, out)
		}
	}
	return nil
}
