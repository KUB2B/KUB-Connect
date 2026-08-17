package netcfg

import (
	"errors"
	"fmt"
	"os/exec"
)

// runAll executes a sequence of commands, stopping at the first failure. Shared
// by the Linux (iproute2) and Windows (netsh) routers.
func runAll(cmds [][]string) error {
	for _, cmd := range cmds {
		if err := runCommand(cmd); err != nil {
			return err
		}
	}
	return nil
}

// runAllBestEffort executes every command and joins their errors. Cleanup must
// not stop merely because one route was already absent: later commands may be
// the split-default or physical-interface bypass routes that matter most.
func runAllBestEffort(cmds [][]string) error {
	var errs []error
	for _, cmd := range cmds {
		if err := runCommand(cmd); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func runCommand(cmd []string) error {
	c := exec.Command(cmd[0], cmd[1:]...)
	hideWindow(c) // suppress flashing console windows on Windows (no-op elsewhere)
	if out, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %w: %s", cmd, err, out)
	}
	return nil
}
