package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:         "ClawChannel",
		Width:         1320,
		Height:        840,
		MinWidth:      1080,
		MinHeight:     700,
		DisableResize: false,
		AssetServer:   &assetserver.Options{Assets: assets},
		OnStartup:     app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		log.Fatal(err)
	}
}
