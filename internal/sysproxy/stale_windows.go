package sysproxy

import (
	"fmt"
	"net"
	"time"
)

// ClearStale disables a system proxy left pointing at our local SOCKS port by a
// previous run that died without cleanup (taskkill by the installer, crash, OS
// shutdown). It only touches a "socks=127.0.0.1:<port>" value whose port nobody
// listens on, so a live proxy (ours or another app's) is left alone. Call it
// before connecting. Returns true when it changed something.
//
// ponytail: runs only when the app starts, so after a reboot while connected
// the internet stays broken until the user opens the app. Upgrade path: the
// installer writes HKLM\...\CurrentVersion\Run "KUBConnectProxyGuard" =
// `"<exe>" --clear-proxy` (runs at every logon as that user, so HKCU is right;
// not schtasks — a SYSTEM task sees the wrong HKCU), main.go handles the flag
// by calling ClearStale before wails.Run and exiting, and the uninstaller does
// DeleteRegValue on it.
func ClearStale() (bool, error) {
	k, err := openSettings()
	if err != nil {
		return false, fmt.Errorf("open registry: %w", err)
	}
	defer k.Close()
	s, err := readWindowsProxyState(k)
	if err != nil {
		return false, err
	}
	if !s.enabledExists || s.enabled == 0 {
		return false, nil
	}
	addr, ok := localSocksAddr(s.server)
	if !ok {
		return false, nil
	}
	if c, err := net.DialTimeout("tcp", addr, 300*time.Millisecond); err == nil {
		c.Close()
		return false, nil
	}
	// ponytail: only disables the proxy; the user's pre-connect proxy is not
	// persisted, so a custom proxy they had before is not restored.
	if err := k.SetDWordValue("ProxyEnable", 0); err != nil {
		return false, err
	}
	return true, refreshWinINet()
}
