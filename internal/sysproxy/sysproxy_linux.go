package sysproxy

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type linuxProxy struct {
	saved *linuxProxyState
}

type linuxProxyState struct {
	mode, host, port string // raw GVariant values returned by gsettings get
}

// New returns the Linux (GNOME gsettings) system proxy controller.
func New() Proxy { return &linuxProxy{} }

func setCommands(host string, port int) [][]string {
	return [][]string{
		{"gsettings", "set", "org.gnome.system.proxy.socks", "host", host},
		{"gsettings", "set", "org.gnome.system.proxy.socks", "port", strconv.Itoa(port)},
		{"gsettings", "set", "org.gnome.system.proxy", "mode", "manual"},
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

func getSetting(schema, key string) (string, error) {
	out, err := exec.Command("gsettings", "get", schema, key).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gsettings get %s %s: %w: %s", schema, key, err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

func readLinuxProxyState() (*linuxProxyState, error) {
	mode, err := getSetting("org.gnome.system.proxy", "mode")
	if err != nil {
		return nil, err
	}
	host, err := getSetting("org.gnome.system.proxy.socks", "host")
	if err != nil {
		return nil, err
	}
	port, err := getSetting("org.gnome.system.proxy.socks", "port")
	if err != nil {
		return nil, err
	}
	return &linuxProxyState{mode: mode, host: host, port: port}, nil
}

func restoreCommands(s *linuxProxyState) [][]string {
	return [][]string{
		{"gsettings", "set", "org.gnome.system.proxy.socks", "host", s.host},
		{"gsettings", "set", "org.gnome.system.proxy.socks", "port", s.port},
		{"gsettings", "set", "org.gnome.system.proxy", "mode", s.mode},
	}
}

func (p *linuxProxy) Set(host string, port int) error {
	saved, err := readLinuxProxyState()
	if err != nil {
		return fmt.Errorf("save system proxy: %w", err)
	}
	p.saved = saved
	if err := runAll(setCommands(host, port)); err != nil {
		restoreErr := runAll(restoreCommands(saved))
		p.saved = nil
		return errors.Join(err, restoreErr)
	}
	return nil
}

func (p *linuxProxy) Clear() error {
	if p.saved == nil {
		return nil
	}
	saved := p.saved
	if err := runAll(restoreCommands(saved)); err != nil {
		return err
	}
	p.saved = nil
	return nil
}
