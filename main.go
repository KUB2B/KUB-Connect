//go:build wails

package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/zki/vless-client/internal/privilege"
)

//go:embed all:frontend/dist
var assets embed.FS

// geoAssets carries the geoip.dat/geosite.dat databases xray-core needs for
// geoip:/geosite: routing rules. Extracted to disk on startup (see startup).
//
//go:embed data/geoip.dat data/geosite.dat
var geoAssets embed.FS

func main() {
	// An elevated relaunch is created while the old process still owns Wails'
	// single-instance mutex. Wait for that owner before wails.Run performs its
	// own lock check, otherwise the confirmed UAC launch silently exits.
	if err := privilege.WaitForRelaunchParent(); err != nil {
		log.Printf("elevated relaunch wait: %v", err)
	}
	a := NewApp()
	err := wails.Run(&options.App{
		Title:  "KUB Connect",
		Width:  900,
		Height: 640,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:     a.startup,
		OnShutdown:    a.shutdown,
		OnBeforeClose: a.beforeClose,
		Bind:          []any{a},
		// Guard against multiple instances: a second launch hands off to the
		// already-running process (OnSecondInstanceLaunch) and then exits, so
		// relaunching focuses the existing window instead of spawning a clone.
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "pro.qb2b.kub-connect",
			OnSecondInstanceLaunch: a.onSecondInstanceLaunch,
		},
	})
	if err != nil {
		panic(err)
	}
}
