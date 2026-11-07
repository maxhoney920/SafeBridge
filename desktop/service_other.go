//go:build !darwin

package main

import "errors"

func serviceInstalled() bool { return false }

func installService() error {
	return errors.New("установка службы есть только в версии для macOS; запустите службу вручную: SafeBridge --daemon")
}

func uninstallService() error {
	return errors.New("установка службы есть только в версии для macOS")
}
