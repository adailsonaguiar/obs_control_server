package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:     "OBS Remote Deck",
		Width:     1180,
		Height:    780,
		MinWidth:  920,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 31, G: 31, B: 31, A: 255},
		Windows: &windows.Options{
			Theme: windows.Dark,
			CustomTheme: &windows.ThemeSettings{
				DarkModeTitleBar:           windows.RGB(24, 24, 24),
				DarkModeTitleBarInactive:   windows.RGB(31, 31, 31),
				DarkModeTitleText:          windows.RGB(238, 238, 238),
				DarkModeTitleTextInactive:  windows.RGB(166, 166, 166),
				DarkModeBorder:             windows.RGB(70, 70, 70),
				DarkModeBorderInactive:     windows.RGB(61, 61, 61),
				LightModeTitleBar:          windows.RGB(24, 24, 24),
				LightModeTitleBarInactive:  windows.RGB(31, 31, 31),
				LightModeTitleText:         windows.RGB(238, 238, 238),
				LightModeTitleTextInactive: windows.RGB(166, 166, 166),
				LightModeBorder:            windows.RGB(70, 70, 70),
				LightModeBorderInactive:    windows.RGB(61, 61, 61),
			},
		},
		Mac: &mac.Options{
			Appearance: mac.NSAppearanceNameDarkAqua,
			TitleBar:   mac.TitleBarHiddenInset(),
		},
		OnStartup:     app.startup,
		OnShutdown:    app.shutdown,
		OnBeforeClose: app.beforeClose,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
