//go:build wails

package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"

	"github.com/wailsapp/wails/v2/pkg/options"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/zki/vless-client/internal/app"
	"github.com/zki/vless-client/internal/autostart"
	"github.com/zki/vless-client/internal/firewall"
	"github.com/zki/vless-client/internal/geoassets"
	"github.com/zki/vless-client/internal/netcfg"
	"github.com/zki/vless-client/internal/privilege"
	"github.com/zki/vless-client/internal/store"
	"github.com/zki/vless-client/internal/sysproxy"
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
	ctx          context.Context
	svc          *app.Service
	initErr      string
	stateReady   chan struct{} // closed once persisted state has been loaded (or failed)
	startupReady chan struct{} // closed once geo assets and startup actions are ready
	trayStop     func()        // tears down the tray on shutdown
	// quitting marks an intentional quit (tray "Выход" or the modal "Выйти") so
	// beforeClose lets the close through instead of re-prompting. Wails routes
	// runtime.Quit through OnBeforeClose, so without this flag an always-veto
	// beforeClose would make the app impossible to quit. Atomic because it is set
	// from the systray goroutine and read on the main thread.
	quitting atomic.Bool
}

func NewApp() *App {
	return &App{
		stateReady:   make(chan struct{}),
		startupReady: make(chan struct{}),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	defer close(a.startupReady)

	// A previous run killed while connected in proxy mode (installer taskkill,
	// crash, OS shutdown) leaves the OS proxy aimed at a dead local port and
	// breaks browsing with ERR_PROXY_CONNECTION_FAILED. Nothing is connected
	// yet, so reset it before autoconnect may set it again.
	if cleared, err := sysproxy.ClearStale(); err != nil {
		log.Printf("clear stale proxy: %v", err)
	} else if cleared {
		log.Printf("cleared stale system proxy left by a previous run")
	}

	// Same idea for the kill switch: a crash in TUN mode leaves the firewall
	// table behind (routes die with the TUN device, the drop rule does not), so
	// whitelisted IPs stay blocked. Off is idempotent and a no-op where the kill
	// switch is unsupported. Removing the table needs root.
	if privilege.IsElevated() {
		if err := firewall.New().Off(); err != nil {
			log.Printf("clear stale kill switch: %v", err)
		}
	}

	// Geo databases are comparatively large. Prepare them in parallel with
	// loading state so the frontend can display saved servers immediately. Any
	// operation that starts xray waits for startupReady below.
	geoReady := make(chan struct{})
	go func() {
		defer close(geoReady)
		if os.Getenv("XRAY_LOCATION_ASSET") != "" {
			return
		}
		if dir, err := geoassets.DefaultDir(); err != nil {
			log.Printf("geo asset dir: %v", err)
		} else if err := geoassets.Sync(geoAssets, "data", dir, []string{"geoip.dat", "geosite.dat"}); err != nil {
			log.Printf("geo assets: %v", err)
		} else {
			_ = os.Setenv("XRAY_LOCATION_ASSET", dir)
		}
	}()

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
		DefaultInterface:    netcfg.DefaultInterfaceName,
		Autostart:           autostart.New(),
		OS:                  runtime.GOOS,
		Version:             version,
	})
	if err != nil {
		log.Printf("init service: %v", err)
		a.initErr = err.Error()
		close(a.stateReady)
		<-geoReady
		return
	}
	a.svc = svc
	a.svc.SubscribeLogs(func(line string) {
		wruntime.EventsEmit(a.ctx, "log", line)
	})
	// Publish the fully loaded state before waiting for geo asset extraction.
	// Binding calls synchronize on this close, so they never observe svc == nil
	// merely because Wails started the frontend and OnStartup concurrently.
	close(a.stateReady)
	<-geoReady
	// A pending-connect intent (set before an elevated restart) takes priority
	// over the normal AutoConnect setting; consume it once, else auto-connect.
	if !a.svc.ResumePendingConnect() {
		if a.svc.WantsElevatedAutoConnect() {
			// TUN auto-connect needs admin: relaunch elevated and connect once
			// there (Connect would otherwise fail the privilege check silently).
			// A declined UAC just leaves this instance unprivileged/disconnected.
			if err := a.RelaunchElevated(true); err != nil {
				log.Printf("autoconnect elevate: %v", err)
			}
		} else {
			a.svc.MaybeAutoConnect()
		}
	}
	// Fire-and-forget: refreshing the OS login entry may spawn launchctl on
	// macOS; keep it off the startup path so the window appears promptly.
	go a.svc.ReconcileAutostart()

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

// onSecondInstanceLaunch fires in the first (already-running) instance when the
// user launches the app again. Bring the existing window to the foreground —
// the second process detects the lock and exits on its own — so a relaunch
// surfaces the running app instead of starting a duplicate.
func (a *App) onSecondInstanceLaunch(options.SecondInstanceData) {
	wruntime.WindowShow(a.ctx)
	wruntime.WindowUnminimise(a.ctx)
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
	svc, err := a.service()
	if err != nil {
		return
	}
	if err := svc.Disconnect(); err != nil {
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
func (a *App) HideToTray() {
	if runtime.GOOS == "linux" {
		// No tray on Linux (tray_stub.go): a hidden window could not be brought
		// back, so minimise instead.
		wruntime.WindowMinimise(a.ctx)
		return
	}
	wruntime.WindowHide(a.ctx)
}

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
// When connectAfter is true a one-shot intent is persisted so the elevated
// instance auto-connects once on startup. Returns the error (e.g.
// privilege.ErrElevationDeclined) so the frontend can revert.
func (a *App) RelaunchElevated(connectAfter bool) error {
	svc, svcErr := a.service()
	if svcErr == nil {
		svc.SetPendingConnect(connectAfter)
		if err := svc.Persist(); err != nil {
			svc.SetPendingConnect(false)
			return fmt.Errorf("сохранение перед перезапуском: %w", err)
		}
	}
	if err := privilege.RelaunchElevated(); err != nil {
		if svcErr == nil {
			// Relaunch failed (e.g. UAC declined); this process keeps running, so
			// clear the intent to avoid a spurious auto-connect on the next start.
			svc.SetPendingConnect(false)
			if err := svc.Persist(); err != nil {
				log.Printf("clear pending-connect after declined elevation: %v", err)
			}
		}
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
	if runtime.GOOS != "windows" {
		return UpdateInfo{}
	}
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

// DownloadAndInstall fetches the latest Windows installer, streaming download
// progress to the frontend via "update-progress" events, launches it (UAC),
// then quits this instance so NSIS can overwrite the running exe. Returns an
// error (without quitting) if no installer asset exists, the download fails, or
// the user declines UAC, so the frontend can restore the banner.
func (a *App) DownloadAndInstall() error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("встроенное обновление поддерживается только на Windows")
	}
	rel, err := updater.CheckLatest()
	if err != nil {
		return fmt.Errorf("проверка обновления: %w", err)
	}
	asset, ok := updater.PickInstaller(rel, updater.LegacyWindows())
	if !ok {
		return fmt.Errorf("установщик не найден в релизе %s", rel.TagName)
	}

	// rel.TagName is external (GitHub API JSON); filepath.Base strips any path
	// separators so a hostile tag can't escape TempDir.
	safeTag := filepath.Base(rel.TagName)
	// A fresh private directory: a fixed name in %TEMP% could be pre-created or
	// swapped by another process before the elevated launch.
	dir, err := os.MkdirTemp("", "kub-connect-update-")
	if err != nil {
		return fmt.Errorf("временный каталог: %w", err)
	}
	dst := filepath.Join(dir, fmt.Sprintf("kub-connect-%s-installer.exe", safeTag))
	err = updater.Download(a.ctx, asset, dst, func(done, total int64) {
		wruntime.EventsEmit(a.ctx, "update-progress", map[string]int64{"done": done, "total": total})
	})
	if err != nil {
		return fmt.Errorf("скачивание: %w", err)
	}

	if err := privilege.RunElevated(dst); err != nil {
		return fmt.Errorf("запуск установщика: %w", err)
	}
	a.quit()
	return nil
}

func (a *App) GetState() app.StateDTO {
	svc, err := a.service()
	if err != nil {
		msg := a.initErr
		if msg == "" {
			msg = "сервис не готов"
		}
		return app.StateDTO{
			Servers:      []app.ServerDTO{},
			ActiveServer: -1,
			Settings:     app.SettingsDTO{Mode: string(store.ModeProxy), LogLevel: store.LogNormal},
			Conn:         string(app.ConnError),
			LastError:    "Ошибка инициализации: " + msg,
			Caps:         app.CapsDTO{OS: runtime.GOOS, Version: version},
		}
	}
	return svc.GetState()
}

// service waits until persisted state has been loaded. Wails invokes OnStartup
// concurrently with frontend JavaScript, so a direct nil check here would turn
// normal startup latency into a false empty/error state.
func (a *App) service() (*app.Service, error) {
	<-a.stateReady
	if a.svc != nil {
		return a.svc, nil
	}
	if a.initErr != "" {
		return nil, fmt.Errorf("сервис не готов: %s", a.initErr)
	}
	return nil, fmt.Errorf("сервис не готов")
}

func (a *App) AddServer(link string) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.AddServer(link)
}
func (a *App) RemoveServer(index int) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.RemoveServer(index)
}
func (a *App) SetActiveServer(index int) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.SetActiveServer(index)
}
func (a *App) UpdateProfile(p app.ProfileDTO) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.UpdateProfile(p)
}
func (a *App) Presets() []app.PresetDTO {
	svc, err := a.service()
	if err != nil {
		return nil
	}
	return svc.Presets()
}
func (a *App) UpdateSettings(s app.SettingsDTO) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.UpdateSettings(s)
}
func (a *App) Connect() error {
	// xray needs the geo databases prepared by the remainder of startup.
	<-a.startupReady
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.Connect()
}
func (a *App) Disconnect() error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.Disconnect()
}
func (a *App) Logs() []string {
	svc, err := a.service()
	if err != nil {
		return nil
	}
	return svc.Logs()
}

// Ping measures TCP connect latency to the server at index. Bound to the frontend.
func (a *App) Ping(index int) app.PingResultDTO {
	svc, err := a.service()
	if err != nil {
		return app.PingResultDTO{Error: err.Error()}
	}
	return svc.Ping(index)
}
