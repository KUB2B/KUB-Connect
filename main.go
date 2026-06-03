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

func main() {
	a := NewApp()
	err := wails.Run(&options.App{
		Title:  "VLESS Client",
		Width:  900,
		Height: 640,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: a.startup,
		Bind:      []any{a},
	})
	if err != nil {
		panic(err)
	}
}
