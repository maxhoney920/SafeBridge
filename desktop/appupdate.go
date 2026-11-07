package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"time"

	"github.com/safebridge/desktop/internal/host"
)

// handleCLI — служебные аргументы до запуска окна. true — программа своё сделала.
func handleCLI() bool {
	if slices.Contains(os.Args[1:], "--version") {
		fmt.Println("SafeBridge", version)
		return true
	}
	host.CleanupAppUpdate()
	// перезапуск после обновления или с правами администратора: прежнее окно ещё
	// закрывается и держит блокировку «один экземпляр» — даём ему выйти
	if slices.Contains(os.Args[1:], "--updated") || slices.Contains(os.Args[1:], "--elevated") {
		time.Sleep(2 * time.Second)
	}
	return false
}

// AppCheck — есть ли новая версия приложения (proxyPort — локальный SOCKS ядра:
// через него скачиваем, если GitHub напрямую недоступен).
func (a *App) AppCheck(proxyPort int) *host.AppInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return host.CheckApp(ctx, proxyPort)
}

// AppUpdate скачивает, проверяет и ставит новую версию, затем перезапускает приложение.
func (a *App) AppUpdate(proxyPort int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	path, ver, err := host.UpdateApp(ctx, proxyPort)
	if err != nil {
		return "", err
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("open", "-n", path, "--args", "--updated")
	} else {
		cmd = exec.Command(path, "--updated")
	}
	if err := cmd.Start(); err != nil {
		return ver, fmt.Errorf("версия %s установлена — запустите SafeBridge снова", ver)
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		a.ui.quit()
	}()
	return ver, nil
}
