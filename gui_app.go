//go:build wails

package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/zki/vless-client/internal/app"
	"github.com/zki/vless-client/internal/privilege"
	"github.com/zki/vless-client/internal/store"
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

	statePath, err := store.DefaultPath()
	if err != nil {
		log.Printf("config path: %v", err)
		statePath = "state.json"
	}
	logDir := filepath.Dir(statePath)
	_ = os.MkdirAll(logDir, 0o755)

	svc, err := app.New(app.Deps{
		StatePath: statePath,
		LogDir:    logDir,
		Emitter:   wailsEmitter{ctx},
		Factory:   newConnector,
		Elevated:  privilege.IsElevated,
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

func (a *App) GetState() app.StateDTO                 { return a.svc.GetState() }
func (a *App) AddServer(link string) error            { return a.svc.AddServer(link) }
func (a *App) RemoveServer(index int) error           { return a.svc.RemoveServer(index) }
func (a *App) SetActiveServer(index int) error        { return a.svc.SetActiveServer(index) }
func (a *App) UpdateProfile(p app.ProfileDTO) error   { return a.svc.UpdateProfile(p) }
func (a *App) UpdateSettings(s app.SettingsDTO) error { return a.svc.UpdateSettings(s) }
func (a *App) Connect() error                         { return a.svc.Connect() }
func (a *App) Disconnect() error                      { return a.svc.Disconnect() }
func (a *App) Logs() []string                         { return a.svc.Logs() }
