package netcfg

import (
	"fmt"
	"os/exec"
	"strconv"
)

type linuxRouter struct{}

// New returns the Linux (iproute2) router.
func New() Router { return linuxRouter{} }

func cidr(ip string, prefix int) string {
	return ip + "/" + strconv.Itoa(prefix)
}

func upCommands(c Config) [][]string {
	cmds := [][]string{
		{"ip", "addr", "add", cidr(c.TunIP, c.Prefix), "dev", c.Device},
		{"ip", "link", "set", "dev", c.Device, "up"},
	}
	for _, r := range c.RouteCIDRs {
		cmds = append(cmds, []string{"ip", "route", "add", r, "dev", c.Device})
	}
	return cmds
}

func downCommands(c Config) [][]string {
	var cmds [][]string
	for _, r := range c.RouteCIDRs {
		cmds = append(cmds, []string{"ip", "route", "del", r, "dev", c.Device})
	}
	cmds = append(cmds, []string{"ip", "addr", "del", cidr(c.TunIP, c.Prefix), "dev", c.Device})
	return cmds
}

func runAll(cmds [][]string) error {
	for _, cmd := range cmds {
		if out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %w: %s", cmd, err, out)
		}
	}
	return nil
}

func (linuxRouter) Up(c Config) error   { return runAll(upCommands(c)) }
func (linuxRouter) Down(c Config) error { return runAll(downCommands(c)) }
