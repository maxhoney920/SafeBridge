//go:build !webdev

package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/safebridge/desktop/internal/tray"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend
var embedded embed.FS

func main() {
	if handleCLI() || runDaemonIfRequested() {
		return
	}
	assets, _ := fs.Sub(embedded, "frontend")
	app := NewApp()
	var ctx context.Context
	app.emit = func(name, data string) {
		if ctx != nil {
			wr.EventsEmit(ctx, name, data)
		}
	}
	app.ui = uiHooks{
		openURL: func(u string) { wr.BrowserOpenURL(ctx, u) },
		show: func() {
			wr.Show(ctx)
			wr.WindowUnminimise(ctx)
			wr.WindowShow(ctx)
			if runtime.GOOS == "darwin" {
				tray.FixWindowButtons()
			}
		},
		quit: func() { wr.Quit(ctx) },
	}
	// macOS: своё меню приложения — «Правка» нужна, чтобы работали ⌘C/⌘V (вставка ссылки
	// на подписку), а «Выйти» (⌘Q) завершает приложение, даже когда окно только прячется
	var appMenu *menu.Menu
	if runtime.GOOS == "darwin" {
		appMenu = menu.NewMenu()
		m := appMenu.AddSubmenu("SafeBridge")
		m.AddText("Скрыть SafeBridge", keys.CmdOrCtrl("h"), func(*menu.CallbackData) { wr.Hide(ctx) })
		m.AddText("Свернуть", keys.CmdOrCtrl("m"), func(*menu.CallbackData) { wr.WindowMinimise(ctx) })
		m.AddText("Закрыть окно", keys.CmdOrCtrl("w"), func(*menu.CallbackData) {
			if trayWanted() {
				wr.Hide(ctx) // значок в строке меню остаётся, VPN работает
			} else {
				wr.WindowMinimise(ctx)
			}
		})
		m.AddSeparator()
		m.AddText("Выйти из SafeBridge", keys.CmdOrCtrl("q"), func(*menu.CallbackData) { app.Quit() })
		appMenu.Append(menu.EditMenu())
	}
	err := wails.Run(&options.App{
		Title:            "SafeBridge",
		Width:            400,
		Height:           760,
		MinWidth:         380,
		MinHeight:        560,
		BackgroundColour: &options.RGBA{R: 25, G: 33, B: 44, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        func(c context.Context) { ctx = c },
		OnDomReady: func(context.Context) {
			if runtime.GOOS == "darwin" {
				// на macOS 10.13 кнопки окна пропадали; окно к этому моменту может быть ещё
				// не показано — повторяем через секунду и три
				go func() {
					for _, d := range []time.Duration{0, time.Second, 3 * time.Second} {
						time.Sleep(d)
						log.Println("кнопки окна:", tray.FixWindowButtons())
					}
				}()
			}
		},
		OnShutdown: func(context.Context) { app.shutdown() },
		Menu:       appMenu,
		// со значком в строке меню закрытие окна только прячет SafeBridge (VPN продолжает
		// работать), а выйти можно из меню значка или по ⌘Q
		HideWindowOnClose: trayWanted(),
		Bind:              []interface{}{app},
		// второй запуск не создаёт второе ядро и не спорит за системный прокси — показывает окно
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "safebridge-desktop-2c8f1a",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) { app.ui.show() },
		},
		Windows: &windows.Options{Theme: windows.SystemDefault, WebviewBrowserPath: fixedWebView2()},
		Mac: &mac.Options{
			Appearance: mac.DefaultAppearance, // светлая или тёмная — как в системе
			About:      &mac.AboutInfo{Title: "SafeBridge " + version, Message: "VPN для компьютера.\nСделано с любовью safe-internet ♥"},
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}

// fixedWebView2 — папка WebView2 рядом с программой (фиксированная версия среды
// Microsoft Edge WebView2). Нужна там, где общую среду не установить, например на
// Windows 7 без интернета: туда кладут распакованный WebView2 Fixed Version 109.
func fixedWebView2() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Join(filepath.Dir(exe), "WebView2")
	if _, err := os.Stat(filepath.Join(dir, "msedgewebview2.exe")); err == nil {
		return dir
	}
	return ""
}
