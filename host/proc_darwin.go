//go:build darwin

package main

import "syscall"

// На macOS нет Pdeathsig: осиротевшее ядро добивается по pid-файлу при следующем запуске,
// а при штатном закрытии браузера помощник сам останавливает ядро.
func childAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
