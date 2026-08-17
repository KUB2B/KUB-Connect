package sysproxy

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type darwinProxy struct {
	saved *darwinProxyState
}

type darwinProxyState struct {
	service, host, port string
	enabled             bool
}

// New returns the macOS (networksetup) system proxy controller.
func New() Proxy { return &darwinProxy{} }

func setCommands(service, host string, port int) [][]string {
	return [][]string{
		{"networksetup", "-setsocksfirewallproxy", service, host, strconv.Itoa(port)},
		{"networksetup", "-setsocksfirewallproxystate", service, "on"},
	}
}

func restoreCommands(s *darwinProxyState) [][]string {
	var cmds [][]string
	if s.host != "" && s.port != "" {
		cmds = append(cmds, []string{"networksetup", "-setsocksfirewallproxy", s.service, s.host, s.port})
	}
	state := "off"
	if s.enabled {
		state = "on"
	}
	return append(cmds, []string{"networksetup", "-setsocksfirewallproxystate", s.service, state})
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

func readDarwinProxyState(service string) (*darwinProxyState, error) {
	out, err := exec.Command("networksetup", "-getsocksfirewallproxy", service).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("read SOCKS proxy for %s: %w: %s", service, err, out)
	}
	s := &darwinProxyState{service: service}
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "Enabled":
			s.enabled = strings.EqualFold(strings.TrimSpace(value), "Yes")
		case "Server":
			s.host = strings.TrimSpace(value)
		case "Port":
			s.port = strings.TrimSpace(value)
		}
	}
	return s, nil
}

func (p *darwinProxy) Set(host string, port int) error {
	svc, err := primaryService()
	if err != nil {
		return err
	}
	saved, err := readDarwinProxyState(svc)
	if err != nil {
		return err
	}
	p.saved = saved
	if err := runAll(setCommands(svc, host, port)); err != nil {
		restoreErr := runAll(restoreCommands(saved))
		p.saved = nil
		return errors.Join(err, restoreErr)
	}
	return nil
}

func (p *darwinProxy) Clear() error {
	if p.saved == nil {
		return nil
	}
	if err := runAll(restoreCommands(p.saved)); err != nil {
		return err
	}
	p.saved = nil
	return nil
}
