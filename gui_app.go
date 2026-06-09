//go:build wails

package main

import (
	"context"
	_ "embed"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/zki/vless-client/internal/app"
	"github.com/zki/vless-client/internal/firewall"
	"github.com/zki/vless-client/internal/geoassets"
	"github.com/zki/vless-client/internal/netcfg"
	"github.com/zki/vless-client/internal/privilege"
	"github.com/zki/vless-client/internal/store"
	"github.com/zki/vless-client/internal/tun"
	"github.com/zki/vless-client/internal/updater"
)

// trayIconICO / trayIconPNG are the tray images. Windows uses the .ico; macOS
// uses the .png (energye/systray renders PNG on darwin).
//
//go:embed build/windows/icon.ico
var trayIconICO []byte

//go:embed build/appicon.png
var trayIconPNG []byte

// wailsEmitter implements app.Emitter via the Wails runtime.
type wailsEmitter struct{ ctx context.Context }

func (e wailsEmitter) Emit(event string, data any) {
	wruntime.EventsEmit(e.ctx, event, data)
}

// trayIcon returns the platform-appropriate tray image bytes.
func trayIcon() []byte {
	if runtime.GOOS == "darwin" {
		return trayIconPNG
	}
	return trayIconICO
}

// App is the Wails-bound application object.
type App struct {
	ctx      context.Context
	svc      *app.Service
	trayStop func() // tears down the tray on shutdown
	// quitting marks an intentional quit (tray "Выход" or the modal "Выйти") so
	// beforeClose lets the close through instead of re-prompting. Wails routes
	// runtime.Quit through OnBeforeClose, so without this flag an always-veto
	// beforeClose would make the app impossible to quit. Atomic because it is set
	// from the systray goroutine and read on the main thread.
	quitting atomic.Bool
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Extract embedded geo databases and point xray-core at them, unless the
	// environment already overrides the location (useful for dev).
	if os.Getenv("XRAY_LOCATION_ASSET") == "" {
		if dir, err := geoassets.DefaultDir(); err != nil {
			log.Printf("geo asset dir: %v", err)
		} else if err := geoassets.Sync(geoAssets, "data", dir, []string{"geoip.dat", "geosite.dat"}); err != nil {
			log.Printf("geo assets: %v", err)
		} else {
			_ = os.Setenv("XRAY_LOCATION_ASSET", dir)
		}
	}

	statePath, err := store.DefaultPath()
	if err != nil {
		log.Printf("config path: %v", err)
		statePath = "state.json"
	}
	logDir := filepath.Dir(statePath)
	_ = os.MkdirAll(logDir, 0o755)

	// Diagnostic: capture tun2socks engine logs (per-connection lines, device
	// read errors) to tun.log, since the GUI process has no usable stderr.
	if f, err := os.OpenFile(filepath.Join(logDir, "tun.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err != nil {
		log.Printf("tun log: %v", err)
	} else {
		tun.SetLogWriter(f)
	}

	svc, err := app.New(app.Deps{
		StatePath:           statePath,
		LogDir:              logDir,
		Emitter:             wailsEmitter{ctx},
		Factory:             newConnector,
		Elevated:            privilege.IsElevated,
		KillSwitchSupported: firewall.Supported,
		TUNSupported:        netcfg.Supported,
		OS:                  runtime.GOOS,
		Version:             version,
	})
	if err != nil {
		log.Printf("init service: %v", err)
		return
	}
	a.svc = svc
	a.svc.SubscribeLogs(func(line string) {
		wruntime.EventsEmit(a.ctx, "log", line)
	})
	a.svc.MaybeAutoConnect()

	// Tray: show window, toggle connection, quit. Subscribe to connection
	// state so the toggle label stays in sync (fires once immediately).
	// cancel is intentionally not stored; the subscription lives for the app lifetime.
	_ = a.svc.SubscribeConn(func(c app.ConnState) {
		updateTrayConn(c == app.ConnConnected)
	})
	// Connect/Disconnect run on the systray message-pump goroutine and are
	// synchronous (Connect launches xray + sets the system proxy, ~1-3s), so run
	// them off the pump to keep the tray menu responsive during the transition.
	a.trayStop = startTray(trayIcon(), trayCallbacks{
		onShow: func() {
			wruntime.WindowShow(a.ctx)
			wruntime.WindowUnminimise(a.ctx)
		},
		onConnect:    func() { go func() { _ = a.svc.Connect() }() },
		onDisconnect: func() { go func() { _ = a.svc.Disconnect() }() },
		onQuit:       func() { a.quit() },
	})
}

// shutdown runs when the app is terminating (window closed or quit). It tears
// down an active connection so platform cleanup — notably clearing the system
// SOCKS proxy in proxy mode — runs. Without this, closing the window while
// connected leaves the OS proxy pointed at the dead local port, so browsers
// fail with ERR_PROXY_CONNECTION_FAILED. Disconnect is idempotent, so calling
// it when already disconnected is a no-op.
func (a *App) shutdown(ctx context.Context) {
	if a.trayStop != nil {
		a.trayStop()
	}
	if a.svc == nil {
		return
	}
	if err := a.svc.Disconnect(); err != nil {
		log.Printf("shutdown disconnect: %v", err)
	}
}

// beforeClose runs on every close attempt, including the window close button
// AND programmatic runtime.Quit. For an intentional quit (quitting set) it lets
// the close proceed. Otherwise (user clicked the window X) it cancels the native
// close and asks the frontend to show the minimize-or-quit choice; the frontend
// then calls HideToTray or QuitApp.
func (a *App) beforeClose(ctx context.Context) (preventClose bool) {
	if a.quitting.Load() {
		return false
	}
	wruntime.EventsEmit(a.ctx, "close-requested")
	return true
}

// HideToTray hides the window; the app keeps running and is reachable via the
// tray. Bound to the frontend.
func (a *App) HideToTray() { wruntime.WindowHide(a.ctx) }

// QuitApp quits the whole app (runs shutdown → Disconnect → proxy cleanup).
// Bound to the frontend.
func (a *App) QuitApp() { a.quit() }

// quit marks the quit intentional and asks Wails to terminate. Shared by the
// modal "Выйти" (QuitApp) and the tray "Выход".
func (a *App) quit() {
	a.quitting.Store(true)
	wruntime.Quit(a.ctx)
}

// RelaunchElevated persists state, launches an elevated instance of the app, and
// on success quits this (unprivileged) one via quit (so the close is not vetoed).
// Bound to the frontend; called when the user opts to restart for TUN mode.
// Returns the error (e.g. privilege.ErrElevationDeclined) so the frontend can
// revert to proxy mode.
func (a *App) RelaunchElevated() error {
	if a.svc != nil {
		if err := a.svc.Persist(); err != nil {
			log.Printf("persist before elevate: %v", err)
		}
	}
	if err := privilege.RelaunchElevated(); err != nil {
		return err
	}
	a.quit()
	return nil
}

// UpdateInfo is returned by CheckUpdate. Bound to the frontend.
type UpdateInfo struct {
	Available bool   `json:"available"`
	Version   string `json:"version"`
	URL       string `json:"url"`
}

// CheckUpdate queries GitHub releases and reports whether a newer version is
// available. Safe to call concurrently; network errors return Available=false.
func (a *App) CheckUpdate() UpdateInfo {
	rel, err := updater.CheckLatest()
	if err != nil {
		return UpdateInfo{}
	}
	return UpdateInfo{
		Available: updater.IsNewer(version, rel.TagName),
		Version:   rel.TagName,
		URL:       rel.HTMLURL,
	}
}

func (a *App) GetState() app.StateDTO {
	if a.svc == nil {
		return app.StateDTO{}
	}
	return a.svc.GetState()
}
func (a *App) AddServer(link string) error            { return a.svc.AddServer(link) }
func (a *App) RemoveServer(index int) error           { return a.svc.RemoveServer(index) }
func (a *App) SetActiveServer(index int) error        { return a.svc.SetActiveServer(index) }
func (a *App) UpdateProfile(p app.ProfileDTO) error   { return a.svc.UpdateProfile(p) }
func (a *App) UpdateSettings(s app.SettingsDTO) error { return a.svc.UpdateSettings(s) }
func (a *App) Connect() error                         { return a.svc.Connect() }
func (a *App) Disconnect() error                      { return a.svc.Disconnect() }
func (a *App) Logs() []string {
	if a.svc == nil {
		return nil
	}
	return a.svc.Logs()
}
