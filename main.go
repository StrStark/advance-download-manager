//go:build !server

package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/StrStark/advance-download-manager/internal/core"
	"github.com/StrStark/advance-download-manager/internal/platform"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

func main() {
	m := core.NewManager(core.NewStore(filepath.Join(platform.DataDir(), "state.json")), nil)
	app := NewApp(m, os.Args[1:])

	err := wails.Run(&options.App{
		Title:            "ADM",
		Width:            1240,
		Height:           800,
		MinWidth:         940,
		MinHeight:        580,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 9, G: 12, B: 23, A: 255},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []any{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "io.github.adm.download-manager",
			OnSecondInstanceLaunch: app.onSecondInstance,
		},
		Linux: &linux.Options{
			Icon:             icon,
			ProgramName:      "adm",
			WebviewGpuPolicy: linux.WebviewGpuPolicyOnDemand,
		},
		Windows: &windows.Options{
			Theme: windows.SystemDefault,
		},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarDefault(),
			About: &mac.AboutInfo{
				Title:   "ADM",
				Message: "Advance Download Manager",
				Icon:    icon,
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
