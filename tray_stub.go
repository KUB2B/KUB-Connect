//go:build wails && !windows && !darwin

package main

// trayCallbacks mirrors the real implementation so gui_app.go compiles on Linux.
type trayCallbacks struct {
	onShow       func()
	onConnect    func()
	onDisconnect func()
	onQuit       func()
}

// startTray is a no-op on Linux (dev builds only; release targets Windows/macOS).
func startTray(icon []byte, cb trayCallbacks) (stop func()) { return func() {} }

// updateTrayConn is a no-op on Linux.
func updateTrayConn(connected bool) {}
