//go:build wails && (windows || darwin)

package main

import (
	"sync"

	"github.com/energye/systray"
)

// trayCallbacks are the actions the tray triggers. All run on the tray's own
// thread; each implementation hops back to the Wails runtime / Service as needed.
type trayCallbacks struct {
	onShow       func() // bring the window to the foreground
	onConnect    func() // start the connection
	onDisconnect func() // stop the connection
	onQuit       func() // quit the whole app
}

var (
	trayMu        sync.Mutex
	trayToggle    *systray.MenuItem // single Connect/Disconnect item
	trayConnected bool              // last known connection state
	trayCB        trayCallbacks
	trayEnd       func() // systray external-loop teardown
)

// startTray launches the system tray using the external-loop entry point so it
// coexists with the Wails main loop. icon is the platform-appropriate image
// bytes (.ico on Windows, .png on macOS). Returns a stop function for shutdown.
func startTray(icon []byte, cb trayCallbacks) (stop func()) {
	trayMu.Lock()
	trayCB = cb
	trayMu.Unlock()

	onReady := func() {
		systray.SetIcon(icon)
		systray.SetTitle("KUB Connect")
		systray.SetTooltip("KUB Connect")
		// Left-click shows the window.
		systray.SetOnClick(func(systray.IMenu) { trayCB.onShow() })

		mShow := systray.AddMenuItem("Показать", "Показать окно")
		mShow.Click(func() { trayCB.onShow() })

		trayMu.Lock()
		trayToggle = systray.AddMenuItem("Подключить", "Подключить / отключить")
		trayMu.Unlock()
		trayToggle.Click(onToggleClicked)

		mQuit := systray.AddMenuItem("Выход", "Выйти из приложения")
		mQuit.Click(func() { trayCB.onQuit() })

		// Reflect any state that arrived before the menu existed.
		trayMu.Lock()
		connected := trayConnected
		trayMu.Unlock()
		updateTrayConn(connected)
	}

	start, end := systray.RunWithExternalLoop(onReady, func() {})
	trayMu.Lock()
	trayEnd = end
	trayMu.Unlock()
	start()
	return func() {
		trayMu.Lock()
		e := trayEnd
		trayMu.Unlock()
		if e != nil {
			e()
		}
	}
}

// onToggleClicked dispatches the single toggle item based on last known state.
func onToggleClicked() {
	trayMu.Lock()
	connected := trayConnected
	cb := trayCB
	trayMu.Unlock()
	if connected {
		cb.onDisconnect()
	} else {
		cb.onConnect()
	}
}

// updateTrayConn updates the toggle item's label to match connection state.
// Safe to call before the menu is built (state is stored and applied in onReady).
func updateTrayConn(connected bool) {
	trayMu.Lock()
	trayConnected = connected
	item := trayToggle
	trayMu.Unlock()
	if item == nil {
		return
	}
	if connected {
		item.SetTitle("Отключить")
	} else {
		item.SetTitle("Подключить")
	}
}
