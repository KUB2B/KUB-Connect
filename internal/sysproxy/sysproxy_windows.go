package sysproxy

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type windowsProxy struct{}

// New returns the Windows (WinINet registry) system proxy controller.
func New() Proxy { return windowsProxy{} }

const inetSettingsPath = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

func proxyServerValue(host string, port int) string {
	return fmt.Sprintf("socks=%s:%d", host, port)
}

func openSettings() (registry.Key, error) {
	return registry.OpenKey(registry.CURRENT_USER, inetSettingsPath, registry.SET_VALUE)
}

func (windowsProxy) Set(host string, port int) error {
	k, err := openSettings()
	if err != nil {
		return fmt.Errorf("open registry: %w", err)
	}
	defer k.Close()
	if err := k.SetStringValue("ProxyServer", proxyServerValue(host, port)); err != nil {
		return err
	}
	if err := k.SetDWordValue("ProxyEnable", 1); err != nil {
		return err
	}
	return refreshWinINet()
}

func (windowsProxy) Clear() error {
	k, err := openSettings()
	if err != nil {
		return fmt.Errorf("open registry: %w", err)
	}
	defer k.Close()
	if err := k.SetDWordValue("ProxyEnable", 0); err != nil {
		return err
	}
	return refreshWinINet()
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
