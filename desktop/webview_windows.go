//go:build windows && !webdev

package main

import (
	"syscall"
	"unsafe"

	"github.com/safebridge/desktop/internal/host"
	"github.com/wailsapp/go-webview2/webviewloader"
)

const win7Download = "https://github.com/maxhoney920/SafeBridge/releases/latest"

// webView2Ready — на Windows 7 установщик WebView2 от Microsoft больше не работает
// (последняя версия для неё — 109), поэтому вместо него объясняем, где взять архив
// SafeBridge со встроенным WebView2. Возвращает false, если запускаться дальше нельзя.
func webView2Ready() bool {
	if !host.Win7Build() || fixedWebView2() != "" {
		return true
	}
	if v, err := webviewloader.GetAvailableCoreWebView2BrowserVersionString(""); err == nil && v != "" {
		return true
	}
	const (
		mbYesNo       = 0x04
		mbIconWarning = 0x30
		idYes         = 6
	)
	text := "Для окна SafeBridge нужен компонент Microsoft Edge WebView2, а его установщик " +
		"для Windows 7 Microsoft больше не выпускает.\n\n" +
		"Скачайте архив SafeBridge-…-windows7-x64.zip (или -x86.zip для 32-битной Windows): " +
		"в нём SafeBridge вместе с WebView2. Распакуйте архив в любую папку и запускайте " +
		"SafeBridge.exe оттуда — папка WebView2 должна лежать рядом с программой.\n\n" +
		"Открыть страницу загрузки?"
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString("SafeBridge — нужен WebView2")
	user32 := syscall.NewLazyDLL("user32.dll")
	r, _, _ := user32.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), mbYesNo|mbIconWarning)
	if r == idYes {
		verb, _ := syscall.UTF16PtrFromString("open")
		u, _ := syscall.UTF16PtrFromString(win7Download)
		syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW").Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(u)), 0, 0, 1)
	}
	return false
}
