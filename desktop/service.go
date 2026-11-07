package main

import (
	"errors"
	"os"
	"runtime"
	"time"

	"github.com/safebridge/desktop/internal/tray"
)

// serviceLabel — имя службы SafeBridge в launchd (macOS).
const serviceLabel = "online.internet-safe.safebridge.helper"

// serviceSocket — сокет службы. На macOS ядро всегда работает в службе (режим TUN
// требует прав администратора, а системный прокси там не используется). Переменная
// окружения позволяет проверить ту же схему на Linux.
func serviceSocket() string {
	if p := os.Getenv("SAFEBRIDGE_SERVICE_SOCKET"); p != "" {
		return p
	}
	if runtime.GOOS == "darwin" {
		return "/var/run/" + serviceLabel + ".sock"
	}
	return ""
}

func useService() bool { return serviceSocket() != "" && os.Getenv("SAFEBRIDGE_LOCAL_CORE") == "" }

// Caps — что умеет приложение на этой системе.
func (a *App) Caps() map[string]bool {
	return map[string]bool{
		"service": a.svcc != nil,              // ядро в службе с правами администратора
		"tunOnly": a.tunOnly,                  // только режим «Весь трафик», без системного прокси
		"tray":    tray.Available || fakeTray, // значок в строке меню (macOS)
	}
}

// ServiceStatus — установлена ли служба и подходит ли её версия.
func (a *App) ServiceStatus() map[string]any {
	if a.svcc == nil {
		return map[string]any{"ok": true}
	}
	v, err := a.svcc.Hello()
	res := map[string]any{"ok": err == nil && v == version, "version": v, "want": version, "installed": err == nil || serviceInstalled()}
	if err != nil {
		res["error"] = err.Error()
	}
	return res
}

// ServiceInstall устанавливает или обновляет службу (система спросит пароль администратора).
func (a *App) ServiceInstall() error {
	if a.svcc == nil {
		return errors.New("служба на этой системе не нужна")
	}
	a.svcc.Shutdown()
	if err := installService(); err != nil {
		return err
	}
	for i := 0; i < 40; i++ { // launchd запускает службу не мгновенно
		if v, err := a.svcc.Hello(); err == nil && v == version {
			return nil
		}
		a.svcc.Shutdown()
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("служба установлена, но не ответила. Перезапустите SafeBridge")
}

// ServiceUninstall удаляет службу (VPN перестанет работать до новой установки).
func (a *App) ServiceUninstall() error {
	if a.svcc == nil {
		return errors.New("службы нет")
	}
	a.svcc.Shutdown()
	return uninstallService()
}
