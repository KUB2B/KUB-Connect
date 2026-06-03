package sysproxy

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type darwinProxy struct{}

// New returns the macOS (networksetup) system proxy controller.
func New() Proxy { return darwinProxy{} }

func setCommands(service, host string, port int) [][]string {
	return [][]string{
		{"networksetup", "-setsocksfirewallproxy", service, host, strconv.Itoa(port)},
		{"networksetup", "-setsocksfirewallproxystate", service, "on"},
	}
}

func clearCommands(service string) [][]string {
	return [][]string{
		{"networksetup", "-setsocksfirewallproxystate", service, "off"},
	}
}

// primaryService returns the first active network service (e.g. "Wi-Fi").
func primaryService() (string, error) {
	out, err := exec.Command("networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return "", fmt.Errorf("list network services: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		// Skip the header line and disabled services (prefixed with '*').
		if line == "" || strings.HasPrefix(line, "An asterisk") || strings.HasPrefix(line, "*") {
			continue
		}
		return line, nil
	}
	return "", fmt.Errorf("no active network service found")
}

func runAll(cmds [][]string) error {
	for _, c := range cmds {
		if out, err := exec.Command(c[0], c[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %w: %s", c, err, out)
		}
	}
	return nil
}

func (darwinProxy) Set(host string, port int) error {
	svc, err := primaryService()
	if err != nil {
		return err
	}
	return runAll(setCommands(svc, host, port))
}

func (darwinProxy) Clear() error {
	svc, err := primaryService()
	if err != nil {
		return err
	}
	return runAll(clearCommands(svc))
}
