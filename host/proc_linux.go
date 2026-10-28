//go:build linux

package main

import "syscall"

// На Linux ядро получит SIGTERM, если помощник неожиданно умрёт.
func childAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM, Setpgid: true}
}
