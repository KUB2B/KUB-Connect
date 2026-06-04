//go:build !windows

package netcfg

import "os/exec"

// hideWindow is a no-op off Windows; other platforms have no console flash.
func hideWindow(*exec.Cmd) {}
