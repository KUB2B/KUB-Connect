//go:build wails

package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// geoAssets carries the geoip.dat/geosite.dat databases xray-core needs for
// geoip:/geosite: routing rules. Extracted to disk on startup (see startup).
//
//go:embed data/geoip.dat data/geosite.dat
var geoAssets embed.FS

func main() {
	a := NewApp()
	err := wails.Run(&options.App{
		Title:  "VLESS Client",
		Width:  900,
		Height: 640,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:     a.startup,
		OnShutdown:    a.shutdown,
		OnBeforeClose: a.beforeClose,
		Bind:          []any{a},
	})
	if err != nil {
		panic(err)
	}
}
