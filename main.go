package main

import (
	"embed"
	"log"
	"os"

	"helm-wails/internal/peer"
	"helm-wails/internal/session"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if len(os.Args) > 1 && os.Args[1] == "peers" {
		cli := peer.CLI{}
		if err := cli.Run(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "session-launch" {
		cli := session.LaunchCLI{}
		if err := cli.Run(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "session-watch" {
		cli := session.SessionWatchCLI{}
		if err := cli.Run(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}

	app, err := NewApp()
	if err != nil {
		log.Fatal(err)
	}

	appInfo := currentAppInfo()

	err = wails.Run(&options.App{
		Title:            appInfo.Name,
		Width:            1480,
		Height:           940,
		MinWidth:         1100,
		MinHeight:        720,
		StartHidden:      true,
		BackgroundColour: &options.RGBA{R: 11, G: 12, B: 14, A: 255},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:       app.startup,
		OnDomReady:      app.domReady,
		OnBeforeClose:   app.beforeClose,
		OnShutdown:      app.shutdown,
		Menu:            buildMenu(app),
		CSSDragProperty: "--wails-draggable",
		CSSDragValue:    "drag",
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			Appearance:           mac.NSAppearanceNameDarkAqua,
			WebviewIsTransparent: true,
			WindowIsTranslucent:  false,
			About: &mac.AboutInfo{
				Title:   appInfo.Name,
				Message: "Version " + appInfo.Version,
			},
		},
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
