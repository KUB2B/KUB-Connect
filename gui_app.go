//go:build wails

package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"runtime"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/zki/vless-client/internal/app"
	"github.com/zki/vless-client/internal/firewall"
	"github.com/zki/vless-client/internal/geoassets"
	"github.com/zki/vless-client/internal/netcfg"
	"github.com/zki/vless-client/internal/privilege"
	"github.com/zki/vless-client/internal/store"
	"github.com/zki/vless-client/internal/tun"
)

// wailsEmitter implements app.Emitter via the Wails runtime.
type wailsEmitter struct{ ctx context.Context }

func (e wailsEmitter) Emit(event string, data any) {
	wruntime.EventsEmit(e.ctx, event, data)
}

// App is the Wails-bound application object.
type App struct {
	ctx context.Context
	svc *app.Service
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
