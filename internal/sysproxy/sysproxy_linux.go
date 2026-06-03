package sysproxy

import (
	"fmt"
	"os/exec"
	"strconv"
)

type linuxProxy struct{}

// New returns the Linux (GNOME gsettings) system proxy controller.
func New() Proxy { return linuxProxy{} }

func setCommands(host string, port int) [][]string {
	return [][]string{
		{"gsettings", "set", "org.gnome.system.proxy", "mode", "manual"},
		{"gsettings", "set", "org.gnome.system.proxy.socks", "host", host},
		{"gsettings", "set", "org.gnome.system.proxy.socks", "port", strconv.Itoa(port)},
	}
}

func clearCommands() [][]string {
	return [][]string{
		{"gsettings", "set", "org.gnome.system.proxy", "mode", "none"},
	}
}

func runAll(cmds [][]string) error {
	for _, c := range cmds {
		if out, err := exec.Command(c[0], c[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %w: %s", c, err, out)
		}
	}
	return nil
}

func (linuxProxy) Set(host string, port int) error { return runAll(setCommands(host, port)) }
func (linuxProxy) Clear() error                    { return runAll(clearCommands()) }
