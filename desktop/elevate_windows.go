//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

// relaunchElevated запускает это же приложение «от имени администратора» (появится окно UAC).
func relaunchElevated() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	args, _ := syscall.UTF16PtrFromString("--elevated")
	shell32 := syscall.NewLazyDLL("shell32.dll")
	r, _, _ := shell32.NewProc("ShellExecuteW").Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(args)), 0, 1)
	if r <= 32 {
		return syscall.Errno(r)
	}
	return nil
}
