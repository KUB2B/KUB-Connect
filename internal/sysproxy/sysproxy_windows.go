package sysproxy

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type windowsProxy struct {
	saved *windowsProxyState
}

type windowsProxyState struct {
	enabled       uint64
	enabledExists bool
	server        string
	serverExists  bool
}

// New returns the Windows (WinINet registry) system proxy controller.
func New() Proxy { return &windowsProxy{} }

const inetSettingsPath = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

func proxyServerValue(host string, port int) string {
	return fmt.Sprintf("socks=%s:%d", host, port)
}

func openSettings() (registry.Key, error) {
	return registry.OpenKey(registry.CURRENT_USER, inetSettingsPath, registry.QUERY_VALUE|registry.SET_VALUE)
}

func readWindowsProxyState(k registry.Key) (*windowsProxyState, error) {
	s := &windowsProxyState{}
	if v, _, err := k.GetIntegerValue("ProxyEnable"); err == nil {
		s.enabled, s.enabledExists = v, true
	} else if !errors.Is(err, registry.ErrNotExist) {
		return nil, err
	}
	if v, _, err := k.GetStringValue("ProxyServer"); err == nil {
		s.server, s.serverExists = v, true
	} else if !errors.Is(err, registry.ErrNotExist) {
		return nil, err
	}
	return s, nil
}

func restoreWindowsProxyState(k registry.Key, s *windowsProxyState) error {
	var errs []error
	if s.serverExists {
		errs = append(errs, k.SetStringValue("ProxyServer", s.server))
	} else if err := k.DeleteValue("ProxyServer"); err != nil && !errors.Is(err, registry.ErrNotExist) {
		errs = append(errs, err)
	}
	if s.enabledExists {
		errs = append(errs, k.SetDWordValue("ProxyEnable", uint32(s.enabled)))
	} else if err := k.DeleteValue("ProxyEnable"); err != nil && !errors.Is(err, registry.ErrNotExist) {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (p *windowsProxy) Set(host string, port int) error {
	k, err := openSettings()
	if err != nil {
		return fmt.Errorf("open registry: %w", err)
	}
	defer k.Close()
	saved, err := readWindowsProxyState(k)
	if err != nil {
		return fmt.Errorf("read existing proxy: %w", err)
	}
	p.saved = saved
	if err := k.SetStringValue("ProxyServer", proxyServerValue(host, port)); err != nil {
		p.saved = nil
		return err
	}
	if err := k.SetDWordValue("ProxyEnable", 1); err != nil {
		restoreErr := restoreWindowsProxyState(k, saved)
		p.saved = nil
		return errors.Join(err, restoreErr)
	}
	if err := refreshWinINet(); err != nil {
		restoreErr := restoreWindowsProxyState(k, saved)
		_ = refreshWinINet()
		p.saved = nil
		return errors.Join(err, restoreErr)
	}
	return nil
}

func (p *windowsProxy) Clear() error {
	if p.saved == nil {
		return nil
	}
	k, err := openSettings()
	if err != nil {
		return fmt.Errorf("open registry: %w", err)
	}
	defer k.Close()
	if err := restoreWindowsProxyState(k, p.saved); err != nil {
		return err
	}
	if err := refreshWinINet(); err != nil {
		return err
	}
	p.saved = nil
	return nil
}

// refreshWinINet notifies WinINet that proxy settings changed so they take
// effect without a reboot.
func refreshWinINet() error {
	const (
		internetOptionSettingsChanged = 39
		internetOptionRefresh         = 37
	)
	wininet := windows.NewLazySystemDLL("wininet.dll")
	proc := wininet.NewProc("InternetSetOptionW")
	for _, opt := range []uintptr{internetOptionSettingsChanged, internetOptionRefresh} {
		// InternetSetOptionW(NULL, opt, NULL, 0)
		r, _, err := proc.Call(0, opt, uintptr(unsafe.Pointer(nil)), 0)
		if r == 0 {
			return fmt.Errorf("InternetSetOption(%d): %w", opt, err)
		}
	}
	return nil
}
